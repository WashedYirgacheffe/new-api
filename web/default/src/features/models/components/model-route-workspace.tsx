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
import {
  ArrowDownToLine,
  ArrowUpFromLine,
  Loader2,
  Plus,
  Save,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'

import {
  deleteModelRouteGroup,
  getModelRouteRelations,
  saveModelRouteGroup,
} from '../api'
import type {
  ModelRouteGroup,
  ModelRouteRelations,
  ModelRouteTarget,
} from '../types'

const ROUTE_OPERATIONS = [
  'text.chat',
  'image.generate',
  'video.generate',
] as const

function emptyTarget(priority: number): ModelRouteTarget {
  return {
    client_key: crypto.randomUUID(),
    target_model: '',
    priority,
    tie_breaker: 0,
    enabled: true,
    retryable_error_codes: ['429', '500', '502', '503', '504'],
    compatibility_status: 'compatible',
  }
}

function emptyGroup(modelName: string): ModelRouteGroup {
  return {
    canonical_model: modelName,
    operation: 'image.generate',
    policy: 'lowest_effective_cost_failover',
    enabled: false,
    targets: [emptyTarget(10)],
  }
}

function cloneGroup(group: ModelRouteGroup): ModelRouteGroup {
  return {
    ...group,
    targets: group.targets.map((target) => ({
      ...target,
      client_key: target.client_key || crypto.randomUUID(),
      retryable_error_codes: [...target.retryable_error_codes],
    })),
  }
}

function RouteGroupSummary(props: {
  group: ModelRouteGroup
  direction: 'incoming' | 'outgoing'
  onEdit?: () => void
}) {
  const { t } = useTranslation()
  const Icon =
    props.direction === 'incoming' ? ArrowDownToLine : ArrowUpFromLine

  return (
    <div className='rounded-lg border p-3'>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='min-w-0'>
          <div className='flex items-center gap-2'>
            <Icon className='text-muted-foreground size-4' />
            <code className='truncate text-sm font-medium'>
              {props.group.canonical_model}
            </code>
            <Badge variant={props.group.enabled ? 'secondary' : 'outline'}>
              {props.group.enabled
                ? t('Candidate enabled')
                : t('Candidate disabled')}
            </Badge>
          </div>
          <p className='text-muted-foreground mt-1 text-xs'>
            {props.group.operation} · v{props.group.version || 1} ·{' '}
            {props.group.policy}
          </p>
        </div>
        {props.onEdit && (
          <Button size='sm' variant='outline' onClick={props.onEdit}>
            {t('Edit route')}
          </Button>
        )}
      </div>
      <div className='mt-3 flex flex-wrap gap-2'>
        {props.group.targets.map((target) => (
          <div
            key={target.target_model}
            className='bg-muted/40 rounded-md border px-2.5 py-1.5 text-xs'
          >
            <code>{target.target_model}</code>
            <span className='text-muted-foreground ml-2'>
              P{target.priority}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}

function TargetEditor(props: {
  target: ModelRouteTarget
  index: number
  onChange: (next: ModelRouteTarget) => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const set = <K extends keyof ModelRouteTarget>(
    key: K,
    value: ModelRouteTarget[K]
  ) => {
    props.onChange({ ...props.target, [key]: value })
  }

  return (
    <div className='grid gap-3 border-b py-4 last:border-b-0 lg:grid-cols-[minmax(220px,1fr)_100px_100px_minmax(180px,1fr)_auto]'>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Target model')}</Label>
        <Input
          className='h-8 font-mono text-xs'
          placeholder='deepwl/gpt-image-2-c'
          value={props.target.target_model}
          onChange={(event) => set('target_model', event.target.value)}
        />
      </label>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Priority')}</Label>
        <Input
          className='h-8 text-xs'
          type='number'
          min={0}
          value={props.target.priority}
          onChange={(event) => set('priority', Number(event.target.value))}
        />
      </label>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Tie breaker')}</Label>
        <Input
          className='h-8 text-xs'
          type='number'
          min={0}
          value={props.target.tie_breaker}
          onChange={(event) => set('tie_breaker', Number(event.target.value))}
        />
      </label>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Retryable error codes')}</Label>
        <Input
          className='h-8 text-xs'
          value={props.target.retryable_error_codes.join(', ')}
          onChange={(event) =>
            set(
              'retryable_error_codes',
              event.target.value
                .split(',')
                .map((item) => item.trim())
                .filter(Boolean)
            )
          }
        />
      </label>
      <div className='flex items-end justify-end gap-2'>
        <Switch
          size='sm'
          checked={props.target.enabled}
          onCheckedChange={(checked) => set('enabled', checked)}
          aria-label={t('Enable route target {{number}}', {
            number: props.index + 1,
          })}
        />
        <Button
          type='button'
          size='icon-sm'
          variant='ghost'
          aria-label={t('Delete target')}
          onClick={props.onDelete}
        >
          <Trash2 className='size-4' />
        </Button>
      </div>
    </div>
  )
}

export function ModelRouteWorkspace(props: { modelName: string }) {
  const { t } = useTranslation()
  const [relations, setRelations] = useState<ModelRouteRelations | null>(null)
  const [draft, setDraft] = useState<ModelRouteGroup | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const loadRunIdRef = useRef(0)
  const loadContextRef = useRef('')
  loadContextRef.current = props.modelName

  const load = useCallback(async () => {
    const requestModelName = props.modelName
    const runId = loadRunIdRef.current + 1
    loadRunIdRef.current = runId
    if (!requestModelName) {
      setRelations(null)
      setDraft(null)
      setLoading(false)
      return
    }
    setLoading(true)
    try {
      const response = await getModelRouteRelations(requestModelName)
      if (
        runId !== loadRunIdRef.current ||
        requestModelName !== loadContextRef.current
      ) {
        return
      }
      if (!response.success || !response.data) {
        throw new Error(response.message || 'Request failed')
      }
      setRelations(response.data)
      setDraft((current) => {
        if (current && current.canonical_model === requestModelName) {
          return current
        }
        return response.data?.outgoing[0]
          ? cloneGroup(response.data.outgoing[0])
          : emptyGroup(requestModelName)
      })
    } catch (error) {
      if (
        runId === loadRunIdRef.current &&
        requestModelName === loadContextRef.current
      ) {
        toast.error(
          error instanceof Error ? error.message : t('Request failed')
        )
      }
    } finally {
      if (
        runId === loadRunIdRef.current &&
        requestModelName === loadContextRef.current
      ) {
        setLoading(false)
      }
    }
  }, [props.modelName, t])

  useEffect(() => {
    setRelations(null)
    setDraft(null)
    void load()
  }, [load])

  useEffect(
    () => () => {
      loadRunIdRef.current += 1
      loadContextRef.current = ''
    },
    []
  )

  const availableOperations = ROUTE_OPERATIONS.filter(
    (operation) =>
      operation === draft?.operation ||
      !relations?.outgoing.some((group) => group.operation === operation)
  )

  const startNewRoute = () => {
    const operation = ROUTE_OPERATIONS.find(
      (candidate) =>
        !relations?.outgoing.some((group) => group.operation === candidate)
    )
    if (!operation) {
      toast.error(t('Every supported operation already has a candidate route.'))
      return
    }
    setDraft({ ...emptyGroup(props.modelName), operation })
  }

  const save = async () => {
    if (!draft) return
    if (
      !draft.operation.trim() ||
      draft.targets.length === 0 ||
      draft.targets.some((target) => !target.target_model.trim())
    ) {
      toast.error(t('Operation and every target model are required.'))
      return
    }
    setSaving(true)
    try {
      const response = await saveModelRouteGroup(draft, draft.route_hash)
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      toast.success(t('Candidate model route saved.'))
      setDraft(response.data ? cloneGroup(response.data) : draft)
      await load()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    if (!draft?.route_hash) return
    setSaving(true)
    try {
      const response = await deleteModelRouteGroup(
        draft.canonical_model,
        draft.operation,
        draft.route_hash
      )
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      toast.success(t('Candidate model route deleted.'))
      setDeleteOpen(false)
      setDraft(emptyGroup(props.modelName))
      await load()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setSaving(false)
    }
  }

  if (!props.modelName) {
    return (
      <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
        {t('Load an exact gateway model ID to manage its routes.')}
      </div>
    )
  }

  if (loading && !draft) {
    return (
      <div className='flex h-36 items-center justify-center rounded-lg border'>
        <Loader2 className='size-5 animate-spin' />
      </div>
    )
  }

  return (
    <div className='space-y-4'>
      <Alert>
        <AlertTitle>{t('Candidate route configuration only')}</AlertTitle>
        <AlertDescription>
          {t(
            'These records are not connected to Relay retries or billing. Saving them does not change live traffic.'
          )}
        </AlertDescription>
      </Alert>
      <div className='grid gap-4 lg:grid-cols-2'>
        <section className='rounded-lg border'>
          <header className='flex items-center gap-2 border-b p-4'>
            <ArrowDownToLine className='size-4' />
            <div>
              <h3 className='font-medium'>{t('Incoming routes')}</h3>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t('Candidate groups that list this model as a target.')}
              </p>
            </div>
          </header>
          <div className='space-y-3 p-4'>
            {relations?.incoming.length ? (
              relations.incoming.map((group) => (
                <RouteGroupSummary
                  key={`${group.canonical_model}-${group.operation}`}
                  group={group}
                  direction='incoming'
                />
              ))
            ) : (
              <p className='text-muted-foreground py-6 text-center text-sm'>
                {t('No incoming routes.')}
              </p>
            )}
          </div>
        </section>
        <section className='rounded-lg border'>
          <header className='flex items-center justify-between gap-3 border-b p-4'>
            <div className='flex items-center gap-2'>
              <ArrowUpFromLine className='size-4' />
              <div>
                <h3 className='font-medium'>{t('Outgoing routes')}</h3>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t('Candidate targets stored for this canonical model.')}
                </p>
              </div>
            </div>
            <Button
              size='sm'
              variant='outline'
              disabled={
                ROUTE_OPERATIONS.every((operation) =>
                  relations?.outgoing.some(
                    (group) => group.operation === operation
                  )
                ) || saving
              }
              onClick={startNewRoute}
            >
              <Plus className='size-4' />
              {t('New route group')}
            </Button>
          </header>
          <div className='space-y-3 p-4'>
            {relations?.outgoing.length ? (
              relations.outgoing.map((group) => (
                <RouteGroupSummary
                  key={`${group.canonical_model}-${group.operation}`}
                  group={group}
                  direction='outgoing'
                  onEdit={() => setDraft(cloneGroup(group))}
                />
              ))
            ) : (
              <p className='text-muted-foreground py-6 text-center text-sm'>
                {t('No candidate route groups yet.')}
              </p>
            )}
          </div>
        </section>
      </div>

      {draft && (
        <section className='rounded-lg border'>
          <header className='flex flex-wrap items-center justify-between gap-3 border-b p-4'>
            <div>
              <h3 className='font-medium'>{t('Route editor')}</h3>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t(
                  'Targets are validated and ordered for future routing. Saving does not change live traffic.'
                )}
              </p>
            </div>
            <div className='flex gap-2'>
              {draft.route_hash && (
                <Button
                  size='sm'
                  variant='outline'
                  disabled={saving}
                  onClick={() => setDeleteOpen(true)}
                >
                  <Trash2 className='size-4' />
                  {t('Delete route')}
                </Button>
              )}
              <Button size='sm' disabled={saving} onClick={() => void save()}>
                {saving ? (
                  <Loader2 className='size-4 animate-spin' />
                ) : (
                  <Save className='size-4' />
                )}
                {t('Save route')}
              </Button>
            </div>
          </header>
          <div className='grid gap-4 border-b p-4 sm:grid-cols-2 lg:grid-cols-4'>
            <label className='space-y-1.5'>
              <Label className='text-xs'>{t('Canonical model')}</Label>
              <Input
                className='h-8 font-mono text-xs'
                value={draft.canonical_model}
                disabled
              />
            </label>
            <label className='space-y-1.5'>
              <Label className='text-xs'>{t('Operation')}</Label>
              <NativeSelect
                className='h-8 w-full text-xs'
                value={draft.operation}
                disabled={Boolean(draft.route_hash)}
                onChange={(event) =>
                  setDraft((current) =>
                    current
                      ? { ...current, operation: event.target.value }
                      : current
                  )
                }
              >
                {availableOperations.map((operation) => (
                  <NativeSelectOption key={operation} value={operation}>
                    {operation}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </label>
            <label className='space-y-1.5'>
              <Label className='text-xs'>{t('Policy')}</Label>
              <Input
                className='h-8 font-mono text-xs'
                value={draft.policy}
                disabled
              />
            </label>
            <label className='flex items-end gap-2 pb-1'>
              <Switch size='sm' checked={draft.enabled} disabled />
              <span className='text-xs'>
                {t('Runtime activation unavailable')}
              </span>
            </label>
          </div>
          <div className='px-4'>
            {draft.targets.map((target, index) => (
              <TargetEditor
                key={target.client_key || String(target.id)}
                target={target}
                index={index}
                onChange={(next) =>
                  setDraft((current) =>
                    current
                      ? {
                          ...current,
                          targets: current.targets.map((item, itemIndex) =>
                            itemIndex === index ? next : item
                          ),
                        }
                      : current
                  )
                }
                onDelete={() =>
                  setDraft((current) =>
                    current
                      ? {
                          ...current,
                          targets: current.targets.filter(
                            (_, itemIndex) => itemIndex !== index
                          ),
                        }
                      : current
                  )
                }
              />
            ))}
          </div>
          <div className='p-4'>
            <Button
              type='button'
              size='sm'
              variant='outline'
              onClick={() =>
                setDraft((current) =>
                  current
                    ? {
                        ...current,
                        targets: [
                          ...current.targets,
                          emptyTarget((current.targets.length + 1) * 10),
                        ],
                      }
                    : current
                )
              }
            >
              <Plus className='size-4' />
              {t('Add target')}
            </Button>
          </div>
        </section>
      )}

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('Delete route group')}
        desc={t(
          'Delete this candidate route group? Live traffic is not affected.'
        )}
        confirmText={t('Delete route')}
        destructive
        isLoading={saving}
        handleConfirm={() => void remove()}
      />
    </div>
  )
}
