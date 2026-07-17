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
import { useQuery } from '@tanstack/react-query'
import {
  CircleDollarSign,
  ImageIcon,
  Link2,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  Send,
  Trash2,
  Video,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Progress } from '@/components/ui/progress'
import { Slider } from '@/components/ui/slider'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  getPlaygroundCatalog,
  getPlaygroundVideo,
  getUserGroups,
  quotePlaygroundModel,
  runPlaygroundMedia,
} from '../api'
import {
  buildInitialParameters,
  buildMediaRequest,
  buildMaterialRules,
  buildParameterDescriptors,
  buildQuoteParameters,
  coerceParameterValue,
  extractImageOutputs,
  extractVideoTask,
  isPlaygroundBindingDispatchReady,
  missingRequiredParameter,
  playgroundDispatchPath,
  validateMaterials,
  type MaterialKind,
  type MaterialRule,
  type MaterialValidationIssue,
  type MediaOutput,
  type ParameterDescriptor,
  type VideoTaskState,
} from '../lib/media-contract'
import type {
  ContractObject,
  GroupOption,
  PlaygroundMaterialItem,
  PlaygroundMaterials,
  PlaygroundQuoteData,
} from '../types'

const EMPTY_MATERIALS: PlaygroundMaterials = { image: [], video: [], audio: [] }
const COMPLETE_STATUSES = new Set([
  'completed',
  'complete',
  'succeeded',
  'success',
])
const FAILED_STATUSES = new Set(['failed', 'error', 'cancelled', 'canceled'])

function numberLabel(value: number | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return value.toLocaleString(undefined, { maximumFractionDigits: 6 })
}

function usdLabel(value: number | undefined): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '-'
  return new Intl.NumberFormat(undefined, {
    style: 'currency',
    currency: 'USD',
    maximumFractionDigits: 6,
  }).format(value)
}

function ParameterControl(props: {
  descriptor: ParameterDescriptor
  value: unknown
  disabled: boolean
  onChange: (value: string | number | boolean) => void
}) {
  const { t } = useTranslation()
  const descriptor = props.descriptor
  const label = (
    <div className='flex items-center justify-between gap-2'>
      <Label className='text-xs'>{t(descriptor.label)}</Label>
      {descriptor.required && (
        <span className='text-destructive text-[10px]'>{t('Required')}</span>
      )}
    </div>
  )

  if (descriptor.type === 'boolean' || descriptor.widget === 'switch') {
    return (
      <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2'>
        <span className='text-xs'>{t(descriptor.label)}</span>
        <Switch
          checked={Boolean(props.value)}
          disabled={props.disabled}
          onCheckedChange={props.onChange}
          size='sm'
        />
      </label>
    )
  }

  if (descriptor.enumValues.length > 0) {
    const stringValue = props.value === undefined ? '' : String(props.value)
    return (
      <label className='space-y-1.5'>
        {label}
        <NativeSelect
          className='h-8 w-full text-xs'
          value={stringValue}
          disabled={props.disabled}
          onChange={(event) =>
            props.onChange(coerceParameterValue(descriptor, event.target.value))
          }
        >
          {descriptor.enumValues.map((value) => (
            <NativeSelectOption key={String(value)} value={String(value)}>
              {String(value)}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </label>
    )
  }

  if (
    descriptor.widget === 'slider' &&
    descriptor.minimum !== undefined &&
    descriptor.maximum !== undefined
  ) {
    const numericValue =
      typeof props.value === 'number' ? props.value : descriptor.minimum
    return (
      <div className='space-y-2'>
        {label}
        <div className='flex items-center gap-3'>
          <Slider
            min={descriptor.minimum}
            max={descriptor.maximum}
            step={descriptor.step || 1}
            value={[numericValue]}
            disabled={props.disabled}
            onValueChange={(values) => {
              const value = Array.isArray(values) ? values[0] : values
              if (typeof value === 'number') props.onChange(value)
            }}
          />
          <span className='w-12 text-right font-mono text-xs'>
            {numericValue}
          </span>
        </div>
      </div>
    )
  }

  return (
    <label className='space-y-1.5'>
      {label}
      <Input
        className='h-8 text-xs'
        type={descriptor.type === 'string' ? 'text' : 'number'}
        min={descriptor.minimum}
        max={descriptor.maximum}
        step={descriptor.step}
        value={props.value === undefined ? '' : String(props.value)}
        disabled={props.disabled}
        onChange={(event) =>
          props.onChange(coerceParameterValue(descriptor, event.target.value))
        }
      />
    </label>
  )
}

function emptyMaterial(kind: MaterialKind): PlaygroundMaterialItem {
  return {
    id: crypto.randomUUID(),
    source: '',
    mime_type: `${kind}/*`,
  }
}

function MaterialEditor(props: {
  rule: MaterialRule
  values: PlaygroundMaterialItem[]
  disabled: boolean
  onChange: (values: PlaygroundMaterialItem[]) => void
}) {
  const { t } = useTranslation()
  if (props.rule.maxItems <= 0) return null
  let label = t('Reference audio')
  if (props.rule.kind === 'image') label = t('Reference images')
  if (props.rule.kind === 'video') label = t('Reference videos')

  const updateItem = (
    id: string,
    field: keyof PlaygroundMaterialItem,
    value: string | number | undefined
  ) => {
    props.onChange(
      props.values.map((item) =>
        item.id === id ? { ...item, [field]: value } : item
      )
    )
  }

  return (
    <div className='space-y-2'>
      <div className='flex items-center justify-between gap-2'>
        <Label className='text-xs'>{label}</Label>
        <span className='text-muted-foreground text-[10px]'>
          {props.values.length}/{props.rule.maxItems}
        </span>
      </div>
      <p className='text-muted-foreground text-[10px]'>
        {t('Only public HTTP(S) URLs are sent for URL material contracts.')}
      </p>
      <div className='flex flex-wrap gap-1.5'>
        {props.rule.mimeTypes.map((mimeType) => (
          <Badge
            key={mimeType}
            variant='outline'
            className='font-mono text-[10px]'
          >
            {mimeType}
          </Badge>
        ))}
        {props.rule.maxSizeMb !== undefined && (
          <Badge variant='outline' className='text-[10px]'>
            {t('Maximum {{size}} MB', { size: props.rule.maxSizeMb })}
          </Badge>
        )}
        {props.rule.maxTotalDuration !== undefined && (
          <Badge variant='outline' className='text-[10px]'>
            {t('Maximum total duration {{duration}} seconds', {
              duration: props.rule.maxTotalDuration,
            })}
          </Badge>
        )}
      </div>
      <div className='space-y-2'>
        {props.values.map((item, index) => (
          <div key={item.id} className='space-y-2 rounded-md border p-2'>
            <div className='flex items-center gap-2'>
              <Link2 className='text-muted-foreground size-3.5 shrink-0' />
              <Input
                className='h-8 font-mono text-xs'
                type='url'
                placeholder='https://...'
                value={item.source}
                disabled={props.disabled}
                aria-label={t('Material URL {{number}}', {
                  number: index + 1,
                })}
                onChange={(event) =>
                  updateItem(item.id, 'source', event.target.value)
                }
              />
              <Button
                type='button'
                size='icon-sm'
                variant='ghost'
                disabled={props.disabled}
                aria-label={t('Remove material')}
                onClick={() =>
                  props.onChange(
                    props.values.filter((value) => value.id !== item.id)
                  )
                }
              >
                <Trash2 className='size-3' />
              </Button>
            </div>
          </div>
        ))}
      </div>
      <Button
        type='button'
        size='sm'
        variant='outline'
        className='w-full'
        disabled={props.disabled || props.values.length >= props.rule.maxItems}
        onClick={() =>
          props.onChange([...props.values, emptyMaterial(props.rule.kind)])
        }
      >
        <Plus className='size-3.5' />
        {t('Add material URL')}
      </Button>
    </div>
  )
}

export function MediaPlayground(props: {
  operation: 'image.generate' | 'video.generate'
}) {
  const { t } = useTranslation()
  const [group, setGroup] = useState('')
  const [modelId, setModelId] = useState('')
  const [prompt, setPrompt] = useState('')
  const [parameters, setParameters] = useState<ContractObject>({})
  const [materials, setMaterials] =
    useState<PlaygroundMaterials>(EMPTY_MATERIALS)
  const [quote, setQuote] = useState<PlaygroundQuoteData | null>(null)
  const [outputs, setOutputs] = useState<MediaOutput[]>([])
  const [videoTask, setVideoTask] = useState<VideoTaskState | null>(null)
  const [loadingAction, setLoadingAction] = useState<
    'quote' | 'generate' | null
  >(null)
  const generationAbortRef = useRef<AbortController | null>(null)
  const generationRunIdRef = useRef(0)

  const groupsQuery = useQuery({
    queryKey: ['playground-groups'],
    queryFn: getUserGroups,
  })
  const catalogQuery = useQuery({
    queryKey: ['playground-catalog', group],
    queryFn: () => getPlaygroundCatalog(group),
    enabled: group !== '',
    staleTime: 15_000,
  })

  const groups: GroupOption[] = useMemo(
    () => groupsQuery.data || [],
    [groupsQuery.data]
  )
  const models = useMemo(
    () =>
      (catalogQuery.data?.data?.items || []).filter((model) =>
        model.profile_bindings.some(
          (binding) =>
            binding.operation === props.operation &&
            binding.dispatch_ready &&
            isPlaygroundBindingDispatchReady(binding, model.model_id)
        )
      ),
    [catalogQuery.data?.data?.items, props.operation]
  )
  const selectedModel = models.find((model) => model.model_id === modelId)
  const binding = selectedModel?.profile_bindings.find(
    (item) => item.operation === props.operation
  )
  const descriptors = useMemo(
    () => buildParameterDescriptors(binding?.effective_contract),
    [binding?.effective_contract]
  )
  const materialRules = useMemo(
    () => buildMaterialRules(binding?.effective_contract),
    [binding?.effective_contract]
  )
  const materialIssues = useMemo(
    () => validateMaterials(materialRules, materials),
    [materialRules, materials]
  )
  const hasMaterialControls = Object.values(materialRules).some(
    (rule) => rule.maxItems > 0
  )
  const generationLocked = loadingAction === 'generate'
  const controlsLocked = loadingAction !== null

  const materialIssueMessage = (issue: MaterialValidationIssue): string => {
    const values = { ...issue.values }
    if (values.kind === 'image') values.kind = t('Image')
    if (values.kind === 'video') values.kind = t('Video')
    if (values.kind === 'audio') values.kind = t('Audio')
    return t(issue.key, values)
  }

  useEffect(() => {
    if (!group && groups.length > 0) setGroup(groups[0].value)
  }, [group, groups])

  useEffect(() => {
    const nextModelId = models.some((model) => model.model_id === modelId)
      ? modelId
      : models[0]?.model_id || ''
    if (nextModelId !== modelId) setModelId(nextModelId)
  }, [modelId, models])

  useEffect(() => {
    generationAbortRef.current?.abort()
    generationAbortRef.current = null
    generationRunIdRef.current += 1
    setLoadingAction(null)
    setParameters(buildInitialParameters(binding?.effective_contract))
    setMaterials(EMPTY_MATERIALS)
    setQuote(null)
    setOutputs([])
    setVideoTask(null)
  }, [binding?.contract_hash, group, modelId, props.operation]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(
    () => () => {
      generationAbortRef.current?.abort()
      generationRunIdRef.current += 1
    },
    []
  )

  const requestQuote = useCallback(
    async (signal?: AbortSignal, runId?: number) => {
      if (!group || !modelId || !binding) {
        throw new Error(t('Select a dispatch-ready model first.'))
      }
      const missingParameter = missingRequiredParameter(descriptors, parameters)
      if (missingParameter) {
        throw new Error(
          t('Complete the required parameter: {{parameter}}.', {
            parameter: missingParameter,
          })
        )
      }
      const quoteParameters = buildQuoteParameters(prompt, parameters)
      const response = await quotePlaygroundModel(
        group,
        modelId,
        props.operation,
        quoteParameters,
        signal
      )
      if (
        signal?.aborted ||
        (runId !== undefined && runId !== generationRunIdRef.current)
      ) {
        throw new DOMException('Aborted', 'AbortError')
      }
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Quote failed'))
      }
      if (response.data.contract_hash !== binding.contract_hash) {
        await catalogQuery.refetch()
        throw new Error(
          t('The model contract changed. Review the parameters and try again.')
        )
      }
      setQuote(response.data)
      return response.data
    },
    [
      binding,
      catalogQuery,
      descriptors,
      group,
      modelId,
      parameters,
      prompt,
      props.operation,
      t,
    ]
  )

  const handleQuote = async () => {
    const runId = generationRunIdRef.current
    setLoadingAction('quote')
    try {
      await requestQuote(undefined, runId)
      toast.success(t('Quote refreshed.'))
    } catch (error) {
      if ((error as { name?: string })?.name !== 'AbortError') {
        toast.error(
          error instanceof Error ? t(error.message) : t('Request failed')
        )
      }
    } finally {
      if (runId === generationRunIdRef.current) setLoadingAction(null)
    }
  }

  const pollVideo = async (
    task: VideoTaskState,
    controller: AbortController,
    runId: number,
    requestedGroup: string
  ): Promise<boolean> => {
    if (!task.taskId) {
      throw new Error(t('The video API returned no task ID.'))
    }
    const isCurrentRun = () =>
      !controller.signal.aborted && runId === generationRunIdRef.current
    let current = task
    for (let attempt = 0; attempt < 150; attempt += 1) {
      if (!isCurrentRun()) return false
      const status = current.status.toLowerCase()
      if (COMPLETE_STATUSES.has(status) && current.source) {
        setVideoTask(current)
        return true
      }
      if (FAILED_STATUSES.has(status)) {
        throw new Error(current.error || t('Video generation failed.'))
      }
      await new Promise<void>((resolve) => window.setTimeout(resolve, 2_000))
      if (!isCurrentRun()) return false
      const payload = await getPlaygroundVideo(
        current.taskId,
        requestedGroup,
        props.operation,
        controller.signal
      )
      current = extractVideoTask(payload)
      if (!current.taskId) current.taskId = task.taskId
      if (isCurrentRun()) setVideoTask(current)
    }
    throw new Error(t('Video generation polling timed out.'))
  }

  const handleGenerate = async () => {
    if (!binding || !selectedModel) return
    if (!prompt.trim()) {
      toast.error(t('Enter a prompt first.'))
      return
    }
    if (materialIssues.length > 0) {
      const issue = materialIssues[0]
      toast.error(materialIssueMessage(issue))
      return
    }
    generationAbortRef.current?.abort()
    const controller = new AbortController()
    const runId = generationRunIdRef.current + 1
    generationRunIdRef.current = runId
    generationAbortRef.current = controller
    const requestedGroup = group
    setLoadingAction('generate')
    setOutputs([])
    setVideoTask(null)
    try {
      await requestQuote(controller.signal, runId)
      const body = buildMediaRequest(
        selectedModel.model_id,
        binding,
        prompt.trim(),
        parameters,
        materials
      )
      const path = playgroundDispatchPath(binding, selectedModel.model_id)
      const payload = await runPlaygroundMedia(
        path,
        requestedGroup,
        props.operation,
        body,
        controller.signal
      )
      if (controller.signal.aborted || runId !== generationRunIdRef.current) {
        return
      }
      if (props.operation === 'image.generate') {
        const nextOutputs = extractImageOutputs(payload)
        if (nextOutputs.length === 0) {
          throw new Error(t('The request returned no image output.'))
        }
        setOutputs(nextOutputs)
      } else {
        const task = extractVideoTask(payload)
        setVideoTask(task)
        if (!task.source || !COMPLETE_STATUSES.has(task.status.toLowerCase())) {
          const completed = await pollVideo(
            task,
            controller,
            runId,
            requestedGroup
          )
          if (!completed) return
        }
      }
      if (runId === generationRunIdRef.current) {
        toast.success(t('Generation completed.'))
      }
    } catch (error) {
      if (
        (error as { name?: string })?.name !== 'AbortError' &&
        !controller.signal.aborted &&
        runId === generationRunIdRef.current
      ) {
        toast.error(
          error instanceof Error ? t(error.message) : t('Request failed')
        )
      }
    } finally {
      if (runId === generationRunIdRef.current) {
        generationAbortRef.current = null
        setLoadingAction(null)
      }
    }
  }

  const noCatalog = !catalogQuery.isLoading && group && models.length === 0
  const ResultIcon = props.operation === 'image.generate' ? ImageIcon : Video
  const GenerateIcon = props.operation === 'image.generate' ? Send : Play

  return (
    <div className='grid min-h-0 flex-1 overflow-hidden lg:grid-cols-[320px_minmax(0,1fr)]'>
      <aside className='min-h-0 overflow-y-auto border-r p-4'>
        <div className='space-y-5'>
          <div className='space-y-3'>
            <label className='space-y-1.5'>
              <Label className='text-xs'>{t('Group')}</Label>
              <NativeSelect
                className='h-8 w-full text-xs'
                value={group}
                disabled={groupsQuery.isLoading || controlsLocked}
                onChange={(event) => setGroup(event.target.value)}
              >
                {groups.map((item) => (
                  <NativeSelectOption key={item.value} value={item.value}>
                    {item.label} · x{item.ratio}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className='space-y-1.5'>
              <Label className='text-xs'>{t('Model')}</Label>
              <NativeSelect
                className='h-8 w-full text-xs'
                value={modelId}
                disabled={
                  catalogQuery.isLoading ||
                  models.length === 0 ||
                  controlsLocked
                }
                onChange={(event) => setModelId(event.target.value)}
              >
                {models.map((model) => (
                  <NativeSelectOption
                    key={model.model_id}
                    value={model.model_id}
                  >
                    {model.display_name || model.model_id}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            {selectedModel && binding && (
              <div className='bg-muted/30 rounded-md border p-2.5'>
                <p className='truncate font-mono text-xs'>
                  {selectedModel.model_id}
                </p>
                <div className='mt-2 flex flex-wrap gap-1.5'>
                  <Badge variant='outline'>v{binding.contract_version}</Badge>
                  <Badge variant='outline'>{binding.execution_mode}</Badge>
                  {selectedModel.routable && (
                    <Badge variant='secondary'>{t('Routable')}</Badge>
                  )}
                </div>
              </div>
            )}
          </div>

          {noCatalog && (
            <Alert variant='destructive'>
              <AlertTitle>{t('No available models')}</AlertTitle>
              <AlertDescription>
                {t(
                  'No dispatch-ready contract is published for this mode and group.'
                )}
              </AlertDescription>
            </Alert>
          )}

          {descriptors.length > 0 && (
            <section className='space-y-3 border-t pt-4'>
              <h3 className='text-sm font-medium'>{t('Parameters')}</h3>
              {descriptors.map((descriptor) => (
                <ParameterControl
                  key={descriptor.name}
                  descriptor={descriptor}
                  value={parameters[descriptor.name]}
                  disabled={controlsLocked}
                  onChange={(value) => {
                    setQuote(null)
                    setParameters((current) => ({
                      ...current,
                      [descriptor.name]: value,
                    }))
                  }}
                />
              ))}
            </section>
          )}

          {hasMaterialControls && (
            <section className='space-y-4 border-t pt-4'>
              <h3 className='text-sm font-medium'>{t('Materials')}</h3>
              {(['image', 'video', 'audio'] as const).map((kind) => (
                <MaterialEditor
                  key={kind}
                  rule={materialRules[kind]}
                  values={materials[kind]}
                  disabled={controlsLocked}
                  onChange={(values) =>
                    setMaterials((current) => ({ ...current, [kind]: values }))
                  }
                />
              ))}
              {materialIssues.length > 0 && (
                <Alert variant='destructive'>
                  <AlertTitle>{t('Material requirements not met')}</AlertTitle>
                  <AlertDescription>
                    {materialIssueMessage(materialIssues[0])}
                  </AlertDescription>
                </Alert>
              )}
            </section>
          )}

          <section className='space-y-3 border-t pt-4'>
            <div className='flex items-center justify-between'>
              <h3 className='flex items-center gap-1.5 text-sm font-medium'>
                <CircleDollarSign className='size-4' />
                {t('Quote')}
              </h3>
              <Button
                size='icon-sm'
                variant='ghost'
                disabled={!binding || loadingAction !== null}
                aria-label={t('Refresh quote')}
                onClick={() => void handleQuote()}
              >
                {loadingAction === 'quote' ? (
                  <Loader2 className='size-4 animate-spin' />
                ) : (
                  <RefreshCw className='size-4' />
                )}
              </Button>
            </div>
            {quote ? (
              <div className='grid grid-cols-2 gap-2 text-xs'>
                <div className='rounded-md border p-2'>
                  <p className='text-muted-foreground'>
                    {t('Estimated amount')}
                  </p>
                  <p className='mt-1 font-medium'>
                    {usdLabel(quote.estimated_amount)}
                  </p>
                </div>
                <div className='rounded-md border p-2'>
                  <p className='text-muted-foreground'>
                    {t('Estimated quota')}
                  </p>
                  <p className='mt-1 font-medium'>
                    {numberLabel(quote.estimated_quota)}
                  </p>
                </div>
                <p className='text-muted-foreground col-span-2 text-[10px]'>
                  {quote.effective_group} · {quote.estimate_kind}
                </p>
              </div>
            ) : (
              <p className='text-muted-foreground text-xs'>
                {t('Quote is calculated before every generation.')}
              </p>
            )}
          </section>
        </div>
      </aside>

      <section className='flex min-h-0 flex-col'>
        <div className='flex min-h-0 flex-1 items-center justify-center overflow-y-auto p-5'>
          {outputs.length > 0 && (
            <div className='grid w-full max-w-5xl gap-3 sm:grid-cols-2 xl:grid-cols-3'>
              {outputs.map((output, index) => (
                <img
                  key={output.source}
                  src={output.source}
                  alt={t('Generated output {{number}}', { number: index + 1 })}
                  className='max-h-[70vh] w-full rounded-md border object-contain'
                />
              ))}
            </div>
          )}
          {outputs.length === 0 && videoTask?.source && (
            <video
              src={videoTask.source}
              className='max-h-[70vh] max-w-full rounded-md border'
              controls
              autoPlay
            />
          )}
          {outputs.length === 0 &&
            !videoTask?.source &&
            loadingAction === 'generate' && (
              <div className='w-full max-w-md space-y-4 text-center'>
                <Loader2 className='mx-auto size-7 animate-spin' />
                <div>
                  <p className='font-medium'>
                    {videoTask ? t('Generating video') : t('Generating')}
                  </p>
                  <p className='text-muted-foreground mt-1 text-sm'>
                    {videoTask?.status || t('Waiting for upstream response')}
                  </p>
                </div>
                {videoTask?.progress !== undefined && (
                  <Progress
                    value={
                      videoTask.progress > 1
                        ? videoTask.progress
                        : videoTask.progress * 100
                    }
                  />
                )}
              </div>
            )}
          {outputs.length === 0 &&
            !videoTask?.source &&
            loadingAction !== 'generate' && (
              <div className='text-muted-foreground text-center'>
                <ResultIcon className='mx-auto size-10 opacity-40' />
                <p className='mt-3 text-sm'>
                  {t('Generated results appear here.')}
                </p>
              </div>
            )}
        </div>

        <div className='border-t p-4'>
          <div className='bg-background mx-auto max-w-4xl rounded-lg border p-3 shadow-sm'>
            <Textarea
              className='min-h-24 resize-none border-0 bg-transparent p-0 text-sm shadow-none focus-visible:ring-0'
              placeholder={
                props.operation === 'image.generate'
                  ? t('Describe the image you want to create')
                  : t('Describe the video you want to create')
              }
              value={prompt}
              disabled={generationLocked}
              onChange={(event) => setPrompt(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
                  void handleGenerate()
                }
              }}
            />
            <div className='mt-3 flex items-center justify-between gap-3 border-t pt-3'>
              <p className='text-muted-foreground truncate font-mono text-[10px]'>
                {binding?.contract_hash || t('No active contract')}
              </p>
              <Button
                size='sm'
                disabled={
                  !binding ||
                  !prompt.trim() ||
                  materialIssues.length > 0 ||
                  loadingAction !== null
                }
                onClick={() => void handleGenerate()}
              >
                {loadingAction === 'generate' ? (
                  <Loader2 className='size-4 animate-spin' />
                ) : (
                  <GenerateIcon className='size-4' />
                )}
                {t('Generate')}
              </Button>
            </div>
          </div>
        </div>
      </section>
    </div>
  )
}
