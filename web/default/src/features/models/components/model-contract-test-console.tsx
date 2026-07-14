/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { ExternalLink, FlaskConical, Loader2, Play } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Textarea } from '@/components/ui/textarea'

import {
  getTokenModelProfile,
  quoteTokenModel,
  runTokenModelRequest,
} from '../api'
import type { ModelTokenProfileData, ModelTokenQuoteData } from '../types'

type ModelContractTestConsoleProps = {
  modelName: string
  onModelNameChange: (value: string) => void
}

type ImageOutput = {
  source: string
  mimeType: string
}

function objectValue(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null
}

function parseParameters(value: string): Record<string, unknown> {
  const parsed = JSON.parse(value) as unknown
  const object = objectValue(parsed)
  if (!object) throw new Error('Parameters must be a JSON object.')
  return object
}

function nestedObject(
  value: Record<string, unknown> | null,
  camelKey: string,
  snakeKey: string
): Record<string, unknown> | null {
  return objectValue(value?.[camelKey] || value?.[snakeKey])
}

function extractGeminiImages(payload: unknown): ImageOutput[] {
  const root = objectValue(payload)
  const candidates = Array.isArray(root?.candidates) ? root.candidates : []
  const outputs: ImageOutput[] = []
  for (const rawCandidate of candidates) {
    const candidate = objectValue(rawCandidate)
    const content = objectValue(candidate?.content)
    const parts = Array.isArray(content?.parts) ? content.parts : []
    for (const rawPart of parts) {
      const part = objectValue(rawPart)
      const inlineData = nestedObject(part, 'inlineData', 'inline_data')
      const inlineValue = inlineData?.data
      if (typeof inlineValue === 'string' && inlineValue) {
        let mimeType = 'image/png'
        if (typeof inlineData?.mimeType === 'string') {
          mimeType = inlineData.mimeType
        } else if (typeof inlineData?.mime_type === 'string') {
          mimeType = inlineData.mime_type
        }
        outputs.push({
          source: `data:${mimeType};base64,${inlineValue}`,
          mimeType,
        })
        continue
      }
      const fileData = nestedObject(part, 'fileData', 'file_data')
      const fileUri = fileData?.fileUri || fileData?.file_uri
      if (typeof fileUri === 'string' && fileUri) {
        outputs.push({ source: fileUri, mimeType: 'image' })
        continue
      }
      if (typeof part?.text !== 'string') continue
      for (const match of part.text.matchAll(
        /data:(image\/[a-z0-9.+-]+);base64,([a-z0-9+/=]+)/gi
      )) {
        outputs.push({ source: match[0], mimeType: match[1] })
      }
    }
  }
  return outputs
}

function numberLabel(value: number | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return value.toLocaleString(undefined, { maximumFractionDigits: 6 })
}

export function ModelContractTestConsole(props: ModelContractTestConsoleProps) {
  const { t } = useTranslation()
  const [token, setToken] = useState('')
  const [operation, setOperation] = useState('image.generate')
  const [parametersText, setParametersText] = useState(
    '{\n  "aspect_ratio": "1:1",\n  "resolution": "1K"\n}'
  )
  const [prompt, setPrompt] = useState(
    'A single blue circle centered on a plain white background, no text'
  )
  const [profile, setProfile] = useState<ModelTokenProfileData | null>(null)
  const [quote, setQuote] = useState<ModelTokenQuoteData | null>(null)
  const [outputs, setOutputs] = useState<ImageOutput[]>([])
  const [loadingAction, setLoadingAction] = useState<'quote' | 'run' | null>(
    null
  )

  const validateInput = () => {
    const normalizedToken = token.trim()
    const modelName = props.modelName.trim()
    if (!normalizedToken) throw new Error('Enter an API key for this test.')
    if (!modelName) throw new Error('Enter an exact gateway model ID first.')
    return {
      token: normalizedToken,
      modelName,
      parameters: parseParameters(parametersText),
    }
  }

  const loadContractAndQuote = async () => {
    const input = validateInput()
    const profileResponse = await getTokenModelProfile(
      input.token,
      input.modelName,
      operation
    )
    if (!profileResponse.success || !profileResponse.data) {
      throw new Error(profileResponse.message || 'Request failed')
    }
    const quoteResponse = await quoteTokenModel(
      input.token,
      input.modelName,
      operation,
      input.parameters
    )
    if (!quoteResponse.success || !quoteResponse.data) {
      throw new Error(quoteResponse.message || 'Request failed')
    }
    setProfile(profileResponse.data)
    setQuote(quoteResponse.data)
    return { input, profile: profileResponse.data }
  }

  const handleQuote = async () => {
    setLoadingAction('quote')
    setOutputs([])
    try {
      await loadContractAndQuote()
      toast.success(t('Contract and quote loaded.'))
    } catch (error) {
      toast.error(t(error instanceof Error ? error.message : 'Request failed'))
    } finally {
      setLoadingAction(null)
    }
  }

  const handleRun = async () => {
    setLoadingAction('run')
    setOutputs([])
    try {
      const loaded = await loadContractAndQuote()
      const contract = loaded.profile.effective_contract
      if (!loaded.profile.dispatch_ready) {
        throw new Error('This model contract is not dispatch-ready.')
      }
      if (contract.request_contract?.adapter !== 'gemini-image') {
        throw new Error('Real test currently supports Gemini image contracts.')
      }
      const dispatchPath = contract.dispatch_path?.trim()
      if (!dispatchPath) throw new Error('The contract has no dispatch path.')
      if (!prompt.trim()) throw new Error('Enter a prompt for the real test.')
      const dispatchUrl = new URL(
        dispatchPath.replace('{model}', loaded.input.modelName),
        window.location.origin
      )
      if (dispatchUrl.origin !== window.location.origin) {
        throw new Error(
          'The contract dispatch path must remain on this CarLab API origin.'
        )
      }

      const fieldMap = contract.request_contract.field_map || {}
      const mappedParameters: Record<string, unknown> = {}
      for (const [field, value] of Object.entries(loaded.input.parameters)) {
        mappedParameters[fieldMap[field] || field] = value
      }
      const result = await runTokenModelRequest(
        loaded.input.token,
        `${dispatchUrl.pathname}${dispatchUrl.search}`,
        operation,
        `contract-test-${crypto.randomUUID()}`,
        {
          contents: [{ role: 'user', parts: [{ text: prompt.trim() }] }],
          generationConfig: {
            responseModalities: ['IMAGE', 'TEXT'],
            imageConfig: {
              aspectRatio: mappedParameters.aspectRatio || '1:1',
              imageSize: mappedParameters.imageSize || '1K',
            },
          },
        }
      )
      const generatedOutputs = extractGeminiImages(result)
      if (generatedOutputs.length === 0) {
        throw new Error('The request succeeded but returned no image output.')
      }
      setOutputs(generatedOutputs)
      toast.success(t('Real image test completed.'))
    } catch (error) {
      toast.error(t(error instanceof Error ? error.message : 'Request failed'))
    } finally {
      setLoadingAction(null)
    }
  }

  return (
    <section className='rounded-lg border'>
      <header className='border-b p-4'>
        <div className='flex items-center gap-2'>
          <FlaskConical className='size-4' />
          <h2 className='font-medium'>{t('Contract and pricing test')}</h2>
        </div>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t(
            'The API key stays in this page only. Quote uses the key effective group; real tests consume upstream quota and never retry automatically.'
          )}
        </p>
      </header>

      <div className='grid gap-4 p-4 lg:grid-cols-2'>
        <div className='space-y-4'>
          <label className='space-y-1.5'>
            <Label>{t('Test API key')}</Label>
            <Input
              type='password'
              value={token}
              autoComplete='off'
              placeholder='sk-...'
              onChange={(event) => setToken(event.target.value)}
            />
          </label>
          <label className='space-y-1.5'>
            <Label>{t('Gateway model ID')}</Label>
            <Input
              value={props.modelName}
              placeholder='deepwl/gemini-3-pro-image'
              onChange={(event) => props.onModelNameChange(event.target.value)}
            />
          </label>
          <label className='space-y-1.5'>
            <Label>{t('Operation')}</Label>
            <NativeSelect
              className='w-full'
              value={operation}
              onChange={(event) => setOperation(event.target.value)}
            >
              <NativeSelectOption value='text.chat'>
                text.chat
              </NativeSelectOption>
              <NativeSelectOption value='image.generate'>
                image.generate
              </NativeSelectOption>
              <NativeSelectOption value='video.generate'>
                video.generate
              </NativeSelectOption>
            </NativeSelect>
          </label>
          <label className='space-y-1.5'>
            <Label>{t('Quote parameters')}</Label>
            <Textarea
              className='min-h-28 font-mono text-xs'
              value={parametersText}
              onChange={(event) => setParametersText(event.target.value)}
            />
          </label>
          <label className='space-y-1.5'>
            <Label>{t('Real test prompt')}</Label>
            <Textarea
              value={prompt}
              onChange={(event) => setPrompt(event.target.value)}
            />
          </label>
          <div className='flex flex-wrap gap-2'>
            <Button
              variant='outline'
              disabled={loadingAction !== null}
              onClick={handleQuote}
            >
              {loadingAction === 'quote' ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <FlaskConical className='size-4' />
              )}
              {t('Load contract and quote')}
            </Button>
            <Button disabled={loadingAction !== null} onClick={handleRun}>
              {loadingAction === 'run' ? (
                <Loader2 className='size-4 animate-spin' />
              ) : (
                <Play className='size-4' />
              )}
              {t('Run real Gemini image test')}
            </Button>
          </div>
        </div>

        <div className='space-y-4'>
          {profile ? (
            <div className='grid gap-3 rounded-md border p-4 sm:grid-cols-2'>
              <div>
                <p className='text-muted-foreground text-xs'>{t('Profile')}</p>
                <p className='mt-1 font-mono text-sm'>
                  {profile.profile.profile_key}@{profile.profile.version}
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>{t('Dispatch')}</p>
                <Badge
                  className='mt-1'
                  variant={profile.dispatch_ready ? 'secondary' : 'destructive'}
                >
                  {profile.dispatch_ready ? t('Ready') : t('Not ready')}
                </Badge>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>{t('Adapter')}</p>
                <p className='mt-1 font-mono text-sm'>
                  {profile.effective_contract.request_contract?.adapter || '-'}
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Contract version')}
                </p>
                <p className='mt-1 text-sm'>
                  v{profile.effective_contract.contract_version || '-'}
                </p>
              </div>
            </div>
          ) : (
            <Alert>
              <AlertTitle>{t('No test result yet')}</AlertTitle>
              <AlertDescription>
                {t('Load the contract and quote before publishing a model.')}
              </AlertDescription>
            </Alert>
          )}

          {quote && (
            <div className='grid gap-3 rounded-md border p-4 sm:grid-cols-2'>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Effective group')}
                </p>
                <p className='mt-1 font-medium'>{quote.effective_group}</p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Group ratio')}
                </p>
                <p className='mt-1 font-medium'>
                  x{numberLabel(quote.group_ratio)}
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Base price')}
                </p>
                <p className='mt-1 font-medium'>
                  {numberLabel(quote.base_price)}
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Estimated amount')}
                </p>
                <p className='mt-1 font-medium'>
                  {numberLabel(quote.estimated_amount)}
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Estimated quota')}
                </p>
                <p className='mt-1 font-medium'>
                  {numberLabel(quote.estimated_quota)}
                </p>
              </div>
              <div>
                <p className='text-muted-foreground text-xs'>
                  {t('Estimate kind')}
                </p>
                <p className='mt-1 font-mono text-sm'>{quote.estimate_kind}</p>
              </div>
            </div>
          )}

          {outputs.length > 0 && (
            <div className='rounded-md border p-4'>
              <div className='mb-3 flex items-center justify-between gap-3'>
                <div>
                  <p className='font-medium'>{t('Real output')}</p>
                  <p className='text-muted-foreground text-xs'>
                    {t('{{count}} image output(s)', { count: outputs.length })}
                  </p>
                </div>
                <Badge variant='outline'>{outputs[0].mimeType}</Badge>
              </div>
              <img
                src={outputs[0].source}
                alt={t('Generated model test output')}
                className='max-h-72 w-full rounded-md border object-contain'
              />
            </div>
          )}

          <a
            href='/usage-logs/common'
            className='text-primary inline-flex items-center gap-1 text-sm hover:underline'
          >
            {t('Open usage logs to verify actual settlement')}
            <ExternalLink className='size-3' />
          </a>
        </div>
      </div>
    </section>
  )
}
