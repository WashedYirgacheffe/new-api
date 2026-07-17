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
import { zodResolver } from '@hookform/resolvers/zod'
import { Link } from '@tanstack/react-router'
import {
  ExternalLink,
  FileCode2,
  Link2,
  Loader2,
  PlayCircle,
  Plus,
  Route,
  SlidersHorizontal,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

import {
  getModelOperationBindings,
  getModelOperationProfiles,
  saveModelOperationBinding,
  saveModelOperationProfile,
} from '../api'
import {
  modelOperationBindingFormSchema,
  modelOperationProfileFormSchema,
  type ModelOperationBinding,
  type ModelOperationBindingFormValues,
  type ModelOperationProfile,
  type ModelOperationProfileFormValues,
} from '../types'
import { ModelParameterWorkspace } from './model-parameter-workspace'
import { ModelRouteWorkspace } from './model-route-workspace'

const emptyProfile: ModelOperationProfileFormValues = {
  profile_key: '',
  display_name: '',
  description: '',
  version: 1,
  operation: 'image.generate',
  endpoint_type: 'image-generation',
  execution_mode: 'sync',
  response_contract: 'openai-image-generation-v1',
  status: 'draft',
  input_schema:
    '{\n  "type": "object",\n  "properties": {},\n  "additionalProperties": false\n}',
  ui_schema: '{\n  "order": [],\n  "widgets": {}\n}',
  material_schema: '{}',
  smoke_test: '{}',
}

const emptyBinding: ModelOperationBindingFormValues = {
  model_name: '',
  operation: 'image.generate',
  profile_key: '',
  profile_version: 1,
  overrides: '{}',
  enabled: true,
}

function prettyJson(value: Record<string, unknown> | undefined): string {
  return JSON.stringify(value || {}, null, 2)
}

export function ModelContracts() {
  const { t } = useTranslation()
  const [profiles, setProfiles] = useState<ModelOperationProfile[]>([])
  const [bindings, setBindings] = useState<ModelOperationBinding[]>([])
  const [modelQuery, setModelQuery] = useState('')
  const [loadedModelName, setLoadedModelName] = useState('')
  const [loadingProfiles, setLoadingProfiles] = useState(false)
  const [loadingBindings, setLoadingBindings] = useState(false)
  const [profileOpen, setProfileOpen] = useState(false)
  const [bindingOpen, setBindingOpen] = useState(false)
  const [editingBinding, setEditingBinding] =
    useState<ModelOperationBinding | null>(null)
  const [detailRevision, setDetailRevision] = useState(0)
  const [saving, setSaving] = useState(false)
  const bindingLoadRunIdRef = useRef(0)

  const profileForm = useForm<ModelOperationProfileFormValues>({
    resolver: zodResolver(modelOperationProfileFormSchema),
    defaultValues: emptyProfile,
  })
  const bindingForm = useForm<ModelOperationBindingFormValues>({
    resolver: zodResolver(modelOperationBindingFormSchema),
    defaultValues: emptyBinding,
  })

  const latestProfiles = useMemo(() => {
    const latest = new Map<string, ModelOperationProfile>()
    for (const profile of profiles) {
      const current = latest.get(profile.profile_key)
      if (!current || profile.version > current.version) {
        latest.set(profile.profile_key, profile)
      }
    }
    return [...latest.values()].sort((left, right) =>
      left.profile_key.localeCompare(right.profile_key)
    )
  }, [profiles])

  const loadProfiles = useCallback(async () => {
    setLoadingProfiles(true)
    try {
      const response = await getModelOperationProfiles()
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      setProfiles(response.data?.items || [])
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setLoadingProfiles(false)
    }
  }, [t])

  const loadBindings = useCallback(async () => {
    const modelName = modelQuery.trim()
    if (!modelName) {
      toast.error(t('Enter an exact gateway model ID first.'))
      return
    }
    const runId = bindingLoadRunIdRef.current + 1
    bindingLoadRunIdRef.current = runId
    setLoadingBindings(true)
    try {
      const response = await getModelOperationBindings(modelName)
      if (runId !== bindingLoadRunIdRef.current) return
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      setBindings(response.data || [])
      setLoadedModelName(modelName)
      setDetailRevision((current) => current + 1)
    } catch (error) {
      if (runId === bindingLoadRunIdRef.current) {
        toast.error(
          error instanceof Error ? error.message : t('Request failed')
        )
      }
    } finally {
      if (runId === bindingLoadRunIdRef.current) {
        setLoadingBindings(false)
      }
    }
  }, [modelQuery, t])

  useEffect(() => {
    void loadProfiles()
  }, [loadProfiles])

  const openProfile = (profile?: ModelOperationProfile) => {
    profileForm.reset(
      profile
        ? {
            profile_key: profile.profile_key,
            display_name: profile.display_name,
            description: profile.description || '',
            version: profile.version,
            operation: profile.operation,
            endpoint_type: profile.endpoint_type,
            execution_mode: profile.execution_mode as 'sync' | 'async',
            response_contract: profile.response_contract,
            status: profile.status,
            input_schema: prettyJson(profile.input_schema),
            ui_schema: prettyJson(profile.ui_schema),
            material_schema: prettyJson(profile.material_schema),
            smoke_test: prettyJson(profile.smoke_test),
          }
        : emptyProfile
    )
    setProfileOpen(true)
  }

  const openBinding = (binding?: ModelOperationBinding) => {
    setEditingBinding(binding || null)
    bindingForm.reset(
      binding
        ? {
            model_name: binding.model_name,
            operation: binding.operation,
            profile_key: binding.profile_key,
            profile_version: binding.profile_version,
            overrides: prettyJson(binding.overrides),
            enabled: binding.enabled,
          }
        : { ...emptyBinding, model_name: modelQuery.trim() }
    )
    setBindingOpen(true)
  }

  const submitProfile = async (values: ModelOperationProfileFormValues) => {
    setSaving(true)
    try {
      const response = await saveModelOperationProfile({
        ...values,
        input_schema: JSON.parse(values.input_schema),
        ui_schema: JSON.parse(values.ui_schema),
        material_schema: JSON.parse(values.material_schema),
        smoke_test: JSON.parse(values.smoke_test),
      })
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      toast.success(t('Model profile saved.'))
      setProfileOpen(false)
      await loadProfiles()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setSaving(false)
    }
  }

  const submitBinding = async (values: ModelOperationBindingFormValues) => {
    setSaving(true)
    try {
      if (
        editingBinding &&
        (values.model_name !== editingBinding.model_name ||
          values.operation !== editingBinding.operation)
      ) {
        throw new Error(t('Binding model and operation cannot be changed.'))
      }
      const response = await saveModelOperationBinding({
        ...values,
        overrides: JSON.parse(values.overrides),
        expected_contract_hash: editingBinding?.contract_hash,
      })
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      toast.success(t('Model binding saved.'))
      setBindingOpen(false)
      setEditingBinding(null)
      setModelQuery(values.model_name)
      const refreshed = await getModelOperationBindings(values.model_name)
      if (!refreshed.success) {
        throw new Error(refreshed.message || 'Request failed')
      }
      setBindings(refreshed.data || [])
      setLoadedModelName(values.model_name)
      setDetailRevision((current) => current + 1)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setSaving(false)
    }
  }

  const renderError = (message?: string) =>
    message ? (
      <p className='text-destructive mt-1 text-xs'>{t(message)}</p>
    ) : null

  let profileRows
  if (loadingProfiles) {
    profileRows = (
      <TableRow>
        <TableCell colSpan={6} className='h-28 text-center'>
          <Loader2 className='mx-auto size-5 animate-spin' />
        </TableCell>
      </TableRow>
    )
  } else if (latestProfiles.length === 0) {
    profileRows = (
      <TableRow>
        <TableCell
          colSpan={6}
          className='text-muted-foreground h-28 text-center'
        >
          {t('No capability profiles found.')}
        </TableCell>
      </TableRow>
    )
  } else {
    profileRows = latestProfiles.map((profile) => (
      <TableRow key={`${profile.profile_key}-${profile.version}`}>
        <TableCell className='font-mono'>{profile.profile_key}</TableCell>
        <TableCell>{profile.operation}</TableCell>
        <TableCell>{profile.endpoint_type}</TableCell>
        <TableCell>v{profile.version}</TableCell>
        <TableCell>
          <Badge
            variant={profile.status === 'published' ? 'secondary' : 'outline'}
          >
            {profile.status}
          </Badge>
        </TableCell>
        <TableCell className='text-right'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => openProfile(profile)}
          >
            {t('Edit or version')}
          </Button>
        </TableCell>
      </TableRow>
    ))
  }

  return (
    <div className='flex min-h-0 flex-col gap-4'>
      <Alert>
        <FileCode2 className='size-4' />
        <AlertTitle>
          {t('CarLab API is the source of truth for model contracts.')}
        </AlertTitle>
        <AlertDescription>
          {t(
            'Profiles define reusable fields and UI controls. Bindings attach a versioned profile, request adapter, pricing multipliers, and model-specific overrides to one gateway model.'
          )}{' '}
          {t(
            'TapDash receives read-only contract snapshots; profile and binding changes are only accepted here.'
          )}{' '}
          <a
            href='https://doc.deepwl.cn/zh'
            target='_blank'
            rel='noreferrer'
            className='inline-flex items-center gap-1'
          >
            {t('Open upstream documentation')}
            <ExternalLink className='size-3' />
          </a>
        </AlertDescription>
      </Alert>

      <section className='rounded-lg border'>
        <header className='flex flex-col gap-3 border-b p-4 lg:flex-row lg:items-end lg:justify-between'>
          <div>
            <h2 className='font-medium'>{t('Model contract details')}</h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                'Load one exact gateway model to edit its rendered parameters, evidence, versions, and candidate routes.'
              )}
            </p>
          </div>
          <div className='flex w-full max-w-3xl flex-col gap-2 sm:flex-row'>
            <Input
              value={modelQuery}
              onChange={(event) => setModelQuery(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') void loadBindings()
              }}
              placeholder='deepwl/gpt-image-2'
            />
            <Button
              variant='outline'
              disabled={loadingBindings}
              onClick={() => void loadBindings()}
            >
              {loadingBindings && <Loader2 className='size-4 animate-spin' />}
              {t('Load model')}
            </Button>
            <Button variant='outline' render={<Link to='/playground' />}>
              <PlayCircle className='size-4' />
              {t('Open playground')}
            </Button>
          </div>
        </header>
        <div className='p-4'>
          <Tabs defaultValue='parameters'>
            <TabsList className='mb-4'>
              <TabsTrigger value='parameters'>
                <SlidersHorizontal className='size-4' />
                {t('Model parameters')}
              </TabsTrigger>
              <TabsTrigger value='routes'>
                <Route className='size-4' />
                {t('Candidate routes')}
              </TabsTrigger>
            </TabsList>
            <TabsContent value='parameters'>
              <ModelParameterWorkspace
                key={`parameters-${loadedModelName}-${detailRevision}`}
                modelName={loadedModelName}
              />
            </TabsContent>
            <TabsContent value='routes'>
              <ModelRouteWorkspace
                key={`routes-${loadedModelName}-${detailRevision}`}
                modelName={loadedModelName}
              />
            </TabsContent>
          </Tabs>
        </div>
      </section>

      <section className='rounded-lg border'>
        <header className='flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between'>
          <div>
            <h2 className='font-medium'>{t('Capability profiles')}</h2>
            <p className='text-muted-foreground mt-1 text-sm'>
              {t(
                'Publish a new version instead of changing a profile already used in production.'
              )}
            </p>
          </div>
          <Button size='sm' onClick={() => openProfile()}>
            <Plus className='size-4' />
            {t('Create profile')}
          </Button>
        </header>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Profile key')}</TableHead>
              <TableHead>{t('Operation')}</TableHead>
              <TableHead>{t('Endpoint')}</TableHead>
              <TableHead>{t('Version')}</TableHead>
              <TableHead>{t('Status')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>{profileRows}</TableBody>
        </Table>
      </section>

      <section className='rounded-lg border'>
        <header className='border-b p-4'>
          <div className='flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between'>
            <div>
              <h2 className='font-medium'>{t('Gateway model bindings')}</h2>
              <p className='text-muted-foreground mt-1 text-sm'>
                {t(
                  'Search an exact namespaced gateway ID, then edit the binding used by catalog, quote, and generation dispatch.'
                )}
              </p>
            </div>
            <div className='flex w-full max-w-2xl flex-col gap-2 sm:flex-row'>
              <Input
                value={modelQuery}
                onChange={(event) => setModelQuery(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter') {
                    void loadBindings()
                  }
                }}
                placeholder='deepwl/gpt-image-2'
              />
              <Button
                variant='outline'
                onClick={() => void loadBindings()}
                disabled={loadingBindings}
              >
                {loadingBindings && <Loader2 className='size-4 animate-spin' />}
                {t('Load bindings')}
              </Button>
              <Button
                onClick={() => openBinding()}
                disabled={!modelQuery.trim()}
              >
                <Link2 className='size-4' />
                {t('Add binding')}
              </Button>
            </div>
          </div>
        </header>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t('Model')}</TableHead>
              <TableHead>{t('Operation')}</TableHead>
              <TableHead>{t('Profile')}</TableHead>
              <TableHead>{t('Contract')}</TableHead>
              <TableHead>{t('Enabled')}</TableHead>
              <TableHead className='text-right'>{t('Actions')}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {bindings.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={6}
                  className='text-muted-foreground h-24 text-center'
                >
                  {t('Load a gateway model to inspect its bindings.')}
                </TableCell>
              </TableRow>
            ) : (
              bindings.map((binding) => (
                <TableRow key={`${binding.model_name}-${binding.operation}`}>
                  <TableCell className='font-mono'>
                    {binding.model_name}
                  </TableCell>
                  <TableCell>{binding.operation}</TableCell>
                  <TableCell>
                    {binding.profile_key}@{binding.profile_version}
                  </TableCell>
                  <TableCell className='font-mono'>
                    v{binding.contract_version} ·{' '}
                    {binding.contract_hash.slice(0, 10)}
                  </TableCell>
                  <TableCell>{binding.enabled ? t('Yes') : t('No')}</TableCell>
                  <TableCell className='text-right'>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => openBinding(binding)}
                    >
                      {t('Edit binding')}
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </section>

      <Sheet open={profileOpen} onOpenChange={setProfileOpen}>
        <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>{t('Capability profile')}</SheetTitle>
            <SheetDescription>
              {t(
                'Define the reusable schema, widgets, materials, response parser, and lowest-cost smoke payload.'
              )}
            </SheetDescription>
          </SheetHeader>
          <form
            id='model-profile-form'
            className={sideDrawerFormClassName()}
            onSubmit={profileForm.handleSubmit(submitProfile)}
          >
            <SideDrawerSection>
              <div className='grid gap-4 sm:grid-cols-2'>
                <label className='space-y-1.5'>
                  <Label>{t('Profile key')}</Label>
                  <Input {...profileForm.register('profile_key')} />
                  {renderError(
                    profileForm.formState.errors.profile_key?.message
                  )}
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Display name')}</Label>
                  <Input {...profileForm.register('display_name')} />
                  {renderError(
                    profileForm.formState.errors.display_name?.message
                  )}
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Version')}</Label>
                  <Input
                    type='number'
                    min={1}
                    {...profileForm.register('version', {
                      valueAsNumber: true,
                    })}
                  />
                  {renderError(profileForm.formState.errors.version?.message)}
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Status')}</Label>
                  <NativeSelect
                    className='w-full'
                    {...profileForm.register('status')}
                  >
                    <NativeSelectOption value='draft'>draft</NativeSelectOption>
                    <NativeSelectOption value='published'>
                      published
                    </NativeSelectOption>
                    <NativeSelectOption value='archived'>
                      archived
                    </NativeSelectOption>
                  </NativeSelect>
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Operation')}</Label>
                  <Input {...profileForm.register('operation')} />
                  {renderError(profileForm.formState.errors.operation?.message)}
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Endpoint type')}</Label>
                  <Input {...profileForm.register('endpoint_type')} />
                  {renderError(
                    profileForm.formState.errors.endpoint_type?.message
                  )}
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Execution mode')}</Label>
                  <NativeSelect
                    className='w-full'
                    {...profileForm.register('execution_mode')}
                  >
                    <NativeSelectOption value='sync'>sync</NativeSelectOption>
                    <NativeSelectOption value='async'>async</NativeSelectOption>
                  </NativeSelect>
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Response contract')}</Label>
                  <Input {...profileForm.register('response_contract')} />
                  {renderError(
                    profileForm.formState.errors.response_contract?.message
                  )}
                </label>
              </div>
              <label className='space-y-1.5'>
                <Label>{t('Description')}</Label>
                <Textarea rows={3} {...profileForm.register('description')} />
              </label>
            </SideDrawerSection>
            <SideDrawerSection>
              {(
                [
                  'input_schema',
                  'ui_schema',
                  'material_schema',
                  'smoke_test',
                ] as const
              ).map((field) => (
                <label key={field} className='space-y-1.5'>
                  <Label>{field}</Label>
                  <Textarea
                    className='min-h-44 font-mono text-xs'
                    spellCheck={false}
                    {...profileForm.register(field)}
                  />
                  {renderError(profileForm.formState.errors[field]?.message)}
                </label>
              ))}
            </SideDrawerSection>
          </form>
          <SheetFooter className={sideDrawerFooterClassName()}>
            <Button
              type='button'
              variant='outline'
              onClick={() => setProfileOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' form='model-profile-form' disabled={saving}>
              {saving && <Loader2 className='size-4 animate-spin' />}
              {t('Save')}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      <Sheet
        open={bindingOpen}
        onOpenChange={(open) => {
          setBindingOpen(open)
          if (!open) setEditingBinding(null)
        }}
      >
        <SheetContent className={sideDrawerContentClassName('sm:max-w-3xl')}>
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>{t('Gateway model binding')}</SheetTitle>
            <SheetDescription>
              {t(
                'Attach one published profile version and validated model-specific overrides. Saving a behavior change creates a new contract hash and version.'
              )}
            </SheetDescription>
          </SheetHeader>
          <form
            id='model-binding-form'
            className={sideDrawerFormClassName()}
            onSubmit={bindingForm.handleSubmit(submitBinding)}
          >
            <SideDrawerSection>
              <div className='grid gap-4 sm:grid-cols-2'>
                <label className='space-y-1.5'>
                  <Label>{t('Gateway model ID')}</Label>
                  <Input
                    readOnly={editingBinding !== null}
                    {...bindingForm.register('model_name')}
                  />
                  {renderError(
                    bindingForm.formState.errors.model_name?.message
                  )}
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Operation')}</Label>
                  <Input
                    readOnly={editingBinding !== null}
                    {...bindingForm.register('operation')}
                  />
                  {renderError(bindingForm.formState.errors.operation?.message)}
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Profile key')}</Label>
                  <Input
                    list='model-contract-profile-keys'
                    {...bindingForm.register('profile_key')}
                  />
                  <datalist id='model-contract-profile-keys'>
                    {latestProfiles.map((profile) => (
                      <option
                        key={profile.profile_key}
                        value={profile.profile_key}
                      />
                    ))}
                  </datalist>
                  {renderError(
                    bindingForm.formState.errors.profile_key?.message
                  )}
                </label>
                <label className='space-y-1.5'>
                  <Label>{t('Profile version')}</Label>
                  <Input
                    type='number'
                    min={1}
                    {...bindingForm.register('profile_version', {
                      valueAsNumber: true,
                    })}
                  />
                  {renderError(
                    bindingForm.formState.errors.profile_version?.message
                  )}
                </label>
                <label className='flex items-center gap-2 pt-7'>
                  <input
                    type='checkbox'
                    className='size-4'
                    {...bindingForm.register('enabled')}
                  />
                  <span className='text-sm'>{t('Enable this binding')}</span>
                </label>
              </div>
            </SideDrawerSection>
            <SideDrawerSection>
              <div>
                <Label>{t('Binding overrides')}</Label>
                <p className='text-muted-foreground mt-1 text-sm'>
                  {t(
                    'Use structured JSON for branding, schema restrictions, request adapter, field mapping, dispatch paths, defaults, and pricing multipliers. Executable scripts are not accepted.'
                  )}
                </p>
              </div>
              <Textarea
                className='min-h-[420px] font-mono text-xs'
                spellCheck={false}
                {...bindingForm.register('overrides')}
              />
              {renderError(bindingForm.formState.errors.overrides?.message)}
            </SideDrawerSection>
          </form>
          <SheetFooter className={sideDrawerFooterClassName()}>
            <Button
              type='button'
              variant='outline'
              onClick={() => setBindingOpen(false)}
            >
              {t('Cancel')}
            </Button>
            <Button type='submit' form='model-binding-form' disabled={saving}>
              {saving && <Loader2 className='size-4 animate-spin' />}
              {t('Save binding')}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </div>
  )
}
