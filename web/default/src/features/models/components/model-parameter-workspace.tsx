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
  ExternalLink,
  FileCheck2,
  History,
  Loader2,
  Network,
  Plus,
  RotateCcw,
  Save,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  deleteModelOperationParameterEvidence,
  getModelOperationBindingRevisions,
  getModelOperationBindings,
  getModelOperationParameterEvidence,
  rollbackModelOperationBinding,
  saveModelOperationBinding,
  saveModelOperationParameterEvidence,
} from '../api'
import {
  deriveModelContractCapabilities,
  type MaterialInputCapability,
  type ModelOutputKind,
} from '../lib/model-contract-capabilities'
import type {
  Model,
  ModelContractObject,
  ModelOperationBinding,
  ModelOperationBindingRevision,
  ModelOperationParameterEvidence,
} from '../types'

type ParameterKind = 'string' | 'number' | 'integer' | 'boolean'

type ParameterDraft = {
  clientKey: string
  originalName: string
  name: string
  label: string
  type: ParameterKind
  widget: string
  placement: string
  enumText: string
  defaultText: string
  minimumText: string
  maximumText: string
  stepText: string
  required: boolean
}

type EvidenceDraft = {
  id: number
  field: string
  source_type: ModelOperationParameterEvidence['source_type']
  source_url: string
  source_locator: string
  verification_status: ModelOperationParameterEvidence['verification_status']
  verified_at: string
  notes: string
}

const emptyEvidence: EvidenceDraft = {
  id: 0,
  field: '',
  source_type: 'doc',
  source_url: '',
  source_locator: '',
  verification_status: 'documented',
  verified_at: '',
  notes: '',
}

function objectValue(value: unknown): ModelContractObject {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return value as ModelContractObject
}

function stringMap(value: unknown): Record<string, string> {
  const result: Record<string, string> = {}
  for (const [key, item] of Object.entries(objectValue(value))) {
    if (typeof item === 'string') result[key] = item
  }
  return result
}

function schemaDrafts(binding: ModelOperationBinding): ParameterDraft[] {
  const contract = objectValue(binding.effective_contract)
  const inputSchema = objectValue(contract.input_schema)
  const properties = objectValue(inputSchema.properties)
  const uiSchema = objectValue(contract.ui_schema)
  const labels = stringMap(uiSchema.labels)
  const placements = stringMap(uiSchema.placements)
  const widgets = objectValue(uiSchema.widgets)
  const required = new Set(
    Array.isArray(inputSchema.required)
      ? inputSchema.required.filter(
          (item): item is string => typeof item === 'string'
        )
      : []
  )
  const configuredOrder = Array.isArray(uiSchema.order)
    ? uiSchema.order.filter((item): item is string => typeof item === 'string')
    : []
  const order = [
    ...configuredOrder,
    ...Object.keys(properties).filter(
      (field) => !configuredOrder.includes(field)
    ),
  ]

  return order.flatMap((name) => {
    const schema = objectValue(properties[name])
    const type = schema.type
    if (
      placements[name] === 'prompt' ||
      placements[name] === 'material' ||
      name === 'prompt' ||
      (type !== 'string' &&
        type !== 'number' &&
        type !== 'integer' &&
        type !== 'boolean')
    ) {
      return []
    }
    const widgetValue = widgets[name]
    let widget = ''
    if (typeof widgetValue === 'string') {
      widget = widgetValue
    } else if (typeof objectValue(widgetValue).type === 'string') {
      widget = String(objectValue(widgetValue).type)
    }
    let stepText = ''
    if (typeof schema.multipleOf === 'number') {
      stepText = String(schema.multipleOf)
    } else if (typeof schema.step === 'number') {
      stepText = String(schema.step)
    }
    return [
      {
        clientKey: `${name}-${crypto.randomUUID()}`,
        originalName: name,
        name,
        label: labels[name] || name,
        type,
        widget,
        placement: placements[name] || 'advanced',
        enumText: Array.isArray(schema.enum) ? schema.enum.join(', ') : '',
        defaultText: schema.default === undefined ? '' : String(schema.default),
        minimumText:
          typeof schema.minimum === 'number' ? String(schema.minimum) : '',
        maximumText:
          typeof schema.maximum === 'number' ? String(schema.maximum) : '',
        stepText,
        required: required.has(name),
      },
    ]
  })
}

function coerceValue(value: string, type: ParameterKind): unknown {
  if (value === '') return undefined
  if (type === 'boolean') return value === 'true'
  if (type === 'number' || type === 'integer') {
    const number = Number(value)
    if (!Number.isFinite(number)) return value
    return type === 'integer' ? Math.trunc(number) : number
  }
  return value
}

function remapParameterRecord(
  value: unknown,
  renames: Map<string, string>,
  deletedParameterNames: Set<string>
): ModelContractObject {
  const result: ModelContractObject = {}
  for (const [field, item] of Object.entries(objectValue(value))) {
    if (deletedParameterNames.has(field)) continue
    result[renames.get(field) || field] = item
  }
  return result
}

function remapPricingRule(
  value: unknown,
  renames: Map<string, string>,
  deletedParameterNames: Set<string>
): ModelContractObject {
  const pricingRule = { ...objectValue(value) }
  if (!Array.isArray(pricingRule.multipliers)) return pricingRule
  pricingRule.multipliers = pricingRule.multipliers.flatMap((rawRule) => {
    const rule = objectValue(rawRule)
    const field = typeof rule.field === 'string' ? rule.field : ''
    if (!field || deletedParameterNames.has(field)) return []
    return [{ ...rule, field: renames.get(field) || field }]
  })
  return pricingRule
}

function buildSchemaOverrides(
  binding: ModelOperationBinding,
  parameters: ParameterDraft[],
  deletedParameterNames: Set<string>
): ModelContractObject {
  const contract = objectValue(binding.effective_contract)
  const currentInput = objectValue(contract.input_schema)
  const currentProperties = objectValue(currentInput.properties)
  const currentUi = objectValue(contract.ui_schema)
  const renames = new Map(
    parameters.flatMap((parameter) =>
      parameter.originalName && parameter.originalName !== parameter.name
        ? [[parameter.originalName, parameter.name] as const]
        : []
    )
  )
  const oldWidgets = objectValue(currentUi.widgets)
  const oldRequired = new Set(
    Array.isArray(currentInput.required)
      ? currentInput.required.filter(
          (item): item is string => typeof item === 'string'
        )
      : []
  )
  const configuredOrder = Array.isArray(currentUi.order)
    ? currentUi.order.filter((item): item is string => typeof item === 'string')
    : []
  const properties: ModelContractObject = { ...currentProperties }
  const labels: ModelContractObject = { ...objectValue(currentUi.labels) }
  const widgets: ModelContractObject = { ...oldWidgets }
  const placements: ModelContractObject = {
    ...objectValue(currentUi.placements),
  }
  const required = new Set(oldRequired)

  for (const name of deletedParameterNames) {
    delete properties[name]
    delete labels[name]
    delete widgets[name]
    delete placements[name]
    required.delete(name)
  }

  for (const parameter of parameters) {
    const sourceName = parameter.originalName || parameter.name
    if (parameter.originalName && parameter.originalName !== parameter.name) {
      delete properties[parameter.originalName]
      delete labels[parameter.originalName]
      delete widgets[parameter.originalName]
      delete placements[parameter.originalName]
      required.delete(parameter.originalName)
    }
    const schema: ModelContractObject = {
      ...objectValue(currentProperties[sourceName]),
      type: parameter.type,
    }
    const enumValues = parameter.enumText
      .split(',')
      .map((item) => item.trim())
      .filter(Boolean)
      .map((item) => coerceValue(item, parameter.type))
    if (enumValues.length > 0) {
      schema.enum = enumValues
    } else {
      delete schema.enum
    }
    const defaultValue = coerceValue(parameter.defaultText, parameter.type)
    if (defaultValue !== undefined) {
      schema.default = defaultValue
    } else {
      delete schema.default
    }
    for (const [field, value] of [
      ['minimum', parameter.minimumText],
      ['maximum', parameter.maximumText],
      ['multipleOf', parameter.stepText],
    ] as const) {
      if (value === '') {
        delete schema[field]
        continue
      }
      const number = Number(value)
      if (Number.isFinite(number)) schema[field] = number
    }
    properties[parameter.name] = schema
    labels[parameter.name] = parameter.label || parameter.name
    widgets[parameter.name] =
      parameter.widget || (enumValues.length > 0 ? 'select' : 'text')
    placements[parameter.name] = parameter.placement || 'advanced'
    if (parameter.required) {
      required.add(parameter.name)
    } else {
      required.delete(parameter.name)
    }
  }

  const parameterOrder = parameters.map((parameter) => parameter.name)
  const preservedOrder = configuredOrder.filter((name) =>
    Object.hasOwn(properties, name)
  )
  const remainingProperties = Object.keys(properties).filter(
    (name) => !preservedOrder.includes(name) && !parameterOrder.includes(name)
  )
  const order = [
    ...preservedOrder.filter((name) => !parameterOrder.includes(name)),
    ...parameterOrder,
    ...remainingProperties,
  ]

  return {
    ...binding.overrides,
    schema_mode: 'replace',
    input_schema: {
      ...currentInput,
      type: 'object',
      properties,
      required: [...required],
    },
    ui_schema: {
      ...currentUi,
      order,
      labels,
      widgets,
      placements,
    },
    material_schema: objectValue(contract.material_schema),
    request_contract: {
      ...objectValue(contract.request_contract),
      field_map: remapParameterRecord(
        objectValue(contract.request_contract).field_map,
        renames,
        deletedParameterNames
      ),
      coercions: remapParameterRecord(
        objectValue(contract.request_contract).coercions,
        renames,
        deletedParameterNames
      ),
    },
    parameter_defaults: remapParameterRecord(
      contract.parameter_defaults,
      renames,
      deletedParameterNames
    ),
    parameter_overrides: remapParameterRecord(
      contract.parameter_overrides,
      renames,
      deletedParameterNames
    ),
    pricing_rule: remapPricingRule(
      contract.pricing_rule,
      renames,
      deletedParameterNames
    ),
  }
}

function timestampLabel(value: number): string {
  if (!value) return '-'
  return new Date(value * 1000).toLocaleString()
}

function ParameterRow(props: {
  parameter: ParameterDraft
  onChange: (next: ParameterDraft) => void
  onDelete: () => void
}) {
  const { t } = useTranslation()
  const set = (field: keyof ParameterDraft, value: string | boolean) => {
    props.onChange({ ...props.parameter, [field]: value })
  }

  return (
    <div className='grid gap-3 border-b py-4 last:border-b-0 lg:grid-cols-[1.1fr_1fr_110px_110px_110px_auto]'>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Parameter')}</Label>
        <Input
          className='h-8 font-mono text-xs'
          value={props.parameter.name}
          onChange={(event) => set('name', event.target.value.trim())}
        />
      </label>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Display label')}</Label>
        <Input
          className='h-8 text-xs'
          value={props.parameter.label}
          onChange={(event) => set('label', event.target.value)}
        />
      </label>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Type')}</Label>
        <NativeSelect
          className='h-8 w-full text-xs'
          value={props.parameter.type}
          onChange={(event) => set('type', event.target.value)}
        >
          <NativeSelectOption value='string'>string</NativeSelectOption>
          <NativeSelectOption value='number'>number</NativeSelectOption>
          <NativeSelectOption value='integer'>integer</NativeSelectOption>
          <NativeSelectOption value='boolean'>boolean</NativeSelectOption>
        </NativeSelect>
      </label>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Control')}</Label>
        <NativeSelect
          className='h-8 w-full text-xs'
          value={props.parameter.widget}
          onChange={(event) => set('widget', event.target.value)}
        >
          <NativeSelectOption value='text'>{t('Input')}</NativeSelectOption>
          <NativeSelectOption value='stepper'>
            {t('Stepper')}
          </NativeSelectOption>
          <NativeSelectOption value='select'>
            {t('Dropdown')}
          </NativeSelectOption>
          <NativeSelectOption value='segmented'>
            {t('Segmented')}
          </NativeSelectOption>
          <NativeSelectOption value='slider'>{t('Slider')}</NativeSelectOption>
          <NativeSelectOption value='toggle'>{t('Switch')}</NativeSelectOption>
          <NativeSelectOption value='hidden'>{t('Hidden')}</NativeSelectOption>
        </NativeSelect>
      </label>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Placement')}</Label>
        <NativeSelect
          className='h-8 w-full text-xs'
          value={props.parameter.placement}
          onChange={(event) => set('placement', event.target.value)}
        >
          <NativeSelectOption value='footer'>{t('Footer')}</NativeSelectOption>
          <NativeSelectOption value='batch'>{t('Batch')}</NativeSelectOption>
          <NativeSelectOption value='advanced'>
            {t('Advanced')}
          </NativeSelectOption>
          <NativeSelectOption value='hidden'>{t('Hidden')}</NativeSelectOption>
        </NativeSelect>
      </label>
      <div className='flex items-end justify-end'>
        <Button
          type='button'
          size='icon-sm'
          variant='ghost'
          aria-label={t('Delete parameter')}
          onClick={props.onDelete}
        >
          <Trash2 className='size-4' />
        </Button>
      </div>
      <label className='space-y-1.5 lg:col-span-2'>
        <Label className='text-xs'>{t('Dropdown options')}</Label>
        <Input
          className='h-8 text-xs'
          value={props.parameter.enumText}
          placeholder='1:1, 16:9, 9:16'
          onChange={(event) => set('enumText', event.target.value)}
        />
      </label>
      <label className='space-y-1.5'>
        <Label className='text-xs'>{t('Default')}</Label>
        <Input
          className='h-8 text-xs'
          value={props.parameter.defaultText}
          onChange={(event) => set('defaultText', event.target.value)}
        />
      </label>
      <div className='grid grid-cols-3 gap-2 lg:col-span-2'>
        <label className='space-y-1.5'>
          <Label className='text-xs'>{t('Minimum')}</Label>
          <Input
            className='h-8 text-xs'
            value={props.parameter.minimumText}
            onChange={(event) => set('minimumText', event.target.value)}
          />
        </label>
        <label className='space-y-1.5'>
          <Label className='text-xs'>{t('Maximum')}</Label>
          <Input
            className='h-8 text-xs'
            value={props.parameter.maximumText}
            onChange={(event) => set('maximumText', event.target.value)}
          />
        </label>
        <label className='space-y-1.5'>
          <Label className='text-xs'>{t('Step')}</Label>
          <Input
            className='h-8 text-xs'
            value={props.parameter.stepText}
            onChange={(event) => set('stepText', event.target.value)}
          />
        </label>
      </div>
      <label className='flex items-center gap-2 lg:col-span-6'>
        <Switch
          size='sm'
          checked={props.parameter.required}
          onCheckedChange={(checked) => set('required', checked)}
        />
        <span className='text-xs'>{t('Required parameter')}</span>
      </label>
    </div>
  )
}

const CAPABILITY_KIND_LABEL_KEYS: Record<ModelOutputKind, string> = {
  text: 'Text',
  image: 'Image',
  video: 'Video',
  audio: 'Audio',
  embedding: 'Embedding',
  ranking: 'Ranking',
  unknown: 'Unknown',
}

const MODEL_TYPE_OUTPUT_KINDS: Record<string, ModelOutputKind> = {
  text: 'text',
  image: 'image',
  video: 'video',
  audio: 'audio',
  embedding: 'embedding',
  rerank: 'ranking',
}

function ModelCapabilitySummary(props: {
  model?: Model
  binding: ModelOperationBinding
}) {
  const { t } = useTranslation()
  const capabilities = useMemo(
    () => deriveModelContractCapabilities(props.model, props.binding),
    [props.binding, props.model]
  )
  const hasUpstreamInputs =
    capabilities.prompt.accepted ||
    capabilities.textInput.fields.length > 0 ||
    capabilities.materials.length > 0

  const materialLabel = (material: MaterialInputCapability) => {
    const kind = t(CAPABILITY_KIND_LABEL_KEYS[material.kind])
    if (material.maxItems === null) {
      if (material.minItems === 0) {
        return t('{{kind}} materials: unlimited', { kind })
      }
      return t('{{kind}} materials: at least {{min}}', {
        kind,
        min: material.minItems,
      })
    }
    if (material.minItems === material.maxItems) {
      return t('{{kind}} materials: {{count}}', {
        kind,
        count: material.maxItems,
      })
    }
    return t('{{kind}} materials: {{min}}-{{max}}', {
      kind,
      min: material.minItems,
      max: material.maxItems,
    })
  }

  let modelTypeLabel = t(CAPABILITY_KIND_LABEL_KEYS[capabilities.outputKind])
  if (capabilities.modelType) {
    const normalizedModelType = capabilities.modelType.toLowerCase()
    const modelTypeOutputKind = MODEL_TYPE_OUTPUT_KINDS[normalizedModelType]
    modelTypeLabel = modelTypeOutputKind
      ? t(CAPABILITY_KIND_LABEL_KEYS[modelTypeOutputKind])
      : capabilities.modelType
  }

  let downstreamRule = t('No automatic downstream connection rule.')
  if (capabilities.downstreamRule.kind === 'prompt') {
    downstreamRule = t('May connect to a downstream prompt input.')
  } else if (capabilities.downstreamRule.kind === 'material') {
    downstreamRule = t(
      'May connect to downstream models accepting {{kind}} materials.',
      {
        kind: t(
          CAPABILITY_KIND_LABEL_KEYS[capabilities.downstreamRule.materialKind]
        ),
      }
    )
  }

  return (
    <section
      aria-labelledby='model-io-capability-summary'
      className='bg-muted/30 border-y px-4 py-3'
    >
      <div className='mb-3 flex items-center gap-2'>
        <Network className='size-4' />
        <h3 id='model-io-capability-summary' className='text-sm font-medium'>
          {t('I/O capability summary')}
        </h3>
      </div>
      <div className='grid gap-4 sm:grid-cols-2 xl:grid-cols-4'>
        <div className='min-w-0'>
          <p className='text-muted-foreground text-xs'>{t('Model Type')}</p>
          <p className='mt-1 truncate text-sm font-medium'>{modelTypeLabel}</p>
        </div>
        <div className='min-w-0'>
          <p className='text-muted-foreground text-xs'>
            {t('Accepted upstream inputs')}
          </p>
          <div className='mt-1 flex flex-wrap gap-1.5'>
            {capabilities.prompt.accepted && (
              <Badge variant='outline'>
                {capabilities.prompt.required
                  ? t('Prompt required')
                  : t('Prompt optional')}
              </Badge>
            )}
            {capabilities.textInput.fields.length > 0 && (
              <Badge variant='outline'>
                {capabilities.textInput.required
                  ? t('Text input required')
                  : t('Text input optional')}
              </Badge>
            )}
            {capabilities.materials.map((material) => (
              <Badge key={material.kind} variant='outline'>
                {materialLabel(material)}
              </Badge>
            ))}
            {!hasUpstreamInputs && (
              <span className='text-muted-foreground text-sm'>
                {t('No upstream inputs declared.')}
              </span>
            )}
          </div>
        </div>
        <div className='min-w-0'>
          <p className='text-muted-foreground text-xs'>{t('Output type')}</p>
          <div className='mt-1 flex flex-wrap items-center gap-2'>
            <Badge variant='secondary'>
              {t(CAPABILITY_KIND_LABEL_KEYS[capabilities.outputKind])}
            </Badge>
            <code className='text-muted-foreground text-xs'>
              {props.binding.operation}
            </code>
          </div>
        </div>
        <div className='min-w-0'>
          <p className='text-muted-foreground text-xs'>
            {t('Downstream rule')}
          </p>
          <p className='mt-1 text-sm'>{downstreamRule}</p>
        </div>
      </div>
    </section>
  )
}

export function ModelParameterWorkspace(props: {
  modelName: string
  model?: Model
  showCapabilitySummary?: boolean
}) {
  const { t } = useTranslation()
  const [bindings, setBindings] = useState<ModelOperationBinding[]>([])
  const [operation, setOperation] = useState('')
  const [parameters, setParameters] = useState<ParameterDraft[]>([])
  const [deletedParameterNames, setDeletedParameterNames] = useState<
    Set<string>
  >(new Set())
  const [revisions, setRevisions] = useState<ModelOperationBindingRevision[]>(
    []
  )
  const [evidence, setEvidence] = useState<ModelOperationParameterEvidence[]>(
    []
  )
  const [evidenceDraft, setEvidenceDraft] =
    useState<EvidenceDraft>(emptyEvidence)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState<'publish' | 'evidence' | null>(null)
  const [rollbackRevision, setRollbackRevision] =
    useState<ModelOperationBindingRevision | null>(null)
  const bindingRunIdRef = useRef(0)
  const bindingContextRef = useRef('')
  const governanceRunIdRef = useRef(0)
  const governanceContextRef = useRef('')
  bindingContextRef.current = props.modelName
  governanceContextRef.current = `${props.modelName}\n${operation}`

  const selectedBinding = useMemo(
    () => bindings.find((binding) => binding.operation === operation),
    [bindings, operation]
  )

  const loadGovernance = useCallback(
    async (nextOperation: string) => {
      const requestContext = `${props.modelName}\n${nextOperation}`
      if (requestContext !== governanceContextRef.current) return
      if (!props.modelName || !nextOperation) {
        governanceRunIdRef.current += 1
        setRevisions([])
        setEvidence([])
        return
      }
      const runId = governanceRunIdRef.current + 1
      governanceRunIdRef.current = runId
      try {
        const [revisionResponse, evidenceResponse] = await Promise.all([
          getModelOperationBindingRevisions(props.modelName, nextOperation),
          getModelOperationParameterEvidence(props.modelName, nextOperation),
        ])
        if (
          runId !== governanceRunIdRef.current ||
          requestContext !== governanceContextRef.current
        ) {
          return
        }
        if (!revisionResponse.success || !evidenceResponse.success) {
          throw new Error(
            revisionResponse.message ||
              evidenceResponse.message ||
              t('Request failed')
          )
        }
        setRevisions(revisionResponse.data?.items || [])
        setEvidence(evidenceResponse.data?.items || [])
      } catch (error) {
        if (
          runId === governanceRunIdRef.current &&
          requestContext === governanceContextRef.current
        ) {
          toast.error(
            error instanceof Error ? error.message : t('Request failed')
          )
        }
      }
    },
    [props.modelName, t]
  )

  const load = useCallback(async () => {
    const requestModelName = props.modelName
    const runId = bindingRunIdRef.current + 1
    bindingRunIdRef.current = runId
    if (!requestModelName) {
      setBindings([])
      setOperation('')
      setLoading(false)
      return
    }
    setLoading(true)
    try {
      const response = await getModelOperationBindings(requestModelName)
      if (
        runId !== bindingRunIdRef.current ||
        requestModelName !== bindingContextRef.current
      ) {
        return
      }
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      const items = response.data || []
      const nextOperation = items.some((item) => item.operation === operation)
        ? operation
        : items[0]?.operation || ''
      setBindings(items)
      setOperation(nextOperation)
    } catch (error) {
      if (
        runId === bindingRunIdRef.current &&
        requestModelName === bindingContextRef.current
      ) {
        toast.error(
          error instanceof Error ? error.message : t('Request failed')
        )
      }
    } finally {
      if (
        runId === bindingRunIdRef.current &&
        requestModelName === bindingContextRef.current
      ) {
        setLoading(false)
      }
    }
  }, [operation, props.modelName, t])

  useEffect(() => {
    setBindings([])
    setOperation('')
    void load()
  }, [props.modelName]) // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    governanceRunIdRef.current += 1
    setRevisions([])
    setEvidence([])
    setEvidenceDraft(emptyEvidence)
    setRollbackRevision(null)
    if (!selectedBinding) {
      setParameters([])
      setDeletedParameterNames(new Set())
      return
    }
    setParameters(schemaDrafts(selectedBinding))
    setDeletedParameterNames(new Set())
    void loadGovernance(selectedBinding.operation)
  }, [loadGovernance, selectedBinding])

  useEffect(
    () => () => {
      bindingRunIdRef.current += 1
      bindingContextRef.current = ''
      governanceRunIdRef.current += 1
      governanceContextRef.current = ''
    },
    []
  )

  const publishVersion = async () => {
    if (!selectedBinding) return
    const names = parameters.map((parameter) => parameter.name)
    if (names.some((name) => !name) || new Set(names).size !== names.length) {
      toast.error(t('Parameter names must be non-empty and unique.'))
      return
    }
    const currentProperties = objectValue(
      objectValue(objectValue(selectedBinding.effective_contract).input_schema)
        .properties
    )
    const managedOriginalNames = new Set(
      parameters.map((parameter) => parameter.originalName).filter(Boolean)
    )
    for (const name of deletedParameterNames) managedOriginalNames.add(name)
    const preservedNames = new Set(
      Object.keys(currentProperties).filter(
        (name) => !managedOriginalNames.has(name)
      )
    )
    if (names.some((name) => preservedNames.has(name))) {
      toast.error(t('A parameter name conflicts with a preserved field.'))
      return
    }
    setSaving('publish')
    try {
      const overrides = buildSchemaOverrides(
        selectedBinding,
        parameters,
        deletedParameterNames
      )
      const bindingResponse = await saveModelOperationBinding({
        model_name: selectedBinding.model_name,
        operation: selectedBinding.operation,
        profile_key: selectedBinding.profile_key,
        profile_version: selectedBinding.profile_version,
        overrides,
        enabled: selectedBinding.enabled,
        expected_contract_hash: selectedBinding.contract_hash,
      })
      if (!bindingResponse.success) {
        throw new Error(bindingResponse.message || 'Request failed')
      }
      const renamedEvidence = evidence.filter((item) => {
        const parameter = parameters.find(
          (candidate) => candidate.originalName === item.field
        )
        return Boolean(parameter && parameter.name !== item.field)
      })
      const evidenceResults = await Promise.allSettled(
        renamedEvidence.map((item) => {
          const parameter = parameters.find(
            (candidate) => candidate.originalName === item.field
          )
          return saveModelOperationParameterEvidence({
            ...item,
            field: parameter?.name || item.field,
          })
        })
      )
      if (
        evidenceResults.some(
          (result) => result.status === 'rejected' || !result.value.success
        )
      ) {
        toast.warning(
          t('The contract was published, but some evidence needs a refresh.')
        )
      }
      toast.success(t('Contract version published and applied.'))
      await load()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setSaving(null)
    }
  }

  const saveEvidence = async () => {
    if (!operation || !evidenceDraft.field.trim()) {
      toast.error(t('Select a parameter field first.'))
      return
    }
    setSaving('evidence')
    try {
      const verifiedDate = evidenceDraft.verified_at
        ? Math.floor(new Date(evidenceDraft.verified_at).getTime() / 1000)
        : 0
      const response = await saveModelOperationParameterEvidence({
        id: evidenceDraft.id,
        model_name: props.modelName,
        operation,
        field: evidenceDraft.field.trim(),
        source_type: evidenceDraft.source_type,
        source_url: evidenceDraft.source_url.trim(),
        source_locator: evidenceDraft.source_locator.trim(),
        verification_status: evidenceDraft.verification_status,
        verified_at: Number.isFinite(verifiedDate) ? verifiedDate : 0,
        notes: evidenceDraft.notes.trim(),
      })
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      toast.success(t('Parameter evidence saved.'))
      setEvidenceDraft(emptyEvidence)
      await loadGovernance(operation)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setSaving(null)
    }
  }

  const editEvidence = (item: ModelOperationParameterEvidence) => {
    setEvidenceDraft({
      id: item.id,
      field: item.field,
      source_type: item.source_type,
      source_url: item.source_url || '',
      source_locator: item.source_locator || '',
      verification_status: item.verification_status,
      verified_at: item.verified_at
        ? new Date(item.verified_at * 1000).toISOString().slice(0, 10)
        : '',
      notes: item.notes || '',
    })
  }

  const removeEvidence = async (id: number) => {
    try {
      const response = await deleteModelOperationParameterEvidence(id)
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      toast.success(t('Parameter evidence deleted.'))
      await loadGovernance(operation)
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    }
  }

  const rollback = async () => {
    if (!selectedBinding || !rollbackRevision) return
    setSaving('publish')
    try {
      const response = await rollbackModelOperationBinding(
        props.modelName,
        operation,
        rollbackRevision.revision,
        selectedBinding.contract_hash
      )
      if (!response.success) {
        throw new Error(response.message || 'Request failed')
      }
      toast.success(t('Contract binding rolled back.'))
      setRollbackRevision(null)
      await load()
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Request failed'))
    } finally {
      setSaving(null)
    }
  }

  if (!props.modelName) {
    return (
      <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
        {t('Load an exact gateway model ID to manage its parameters.')}
      </div>
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-end justify-between gap-3 rounded-lg border p-4'>
        <div className='min-w-60 space-y-1.5'>
          <Label>{t('Operation')}</Label>
          <NativeSelect
            className='h-8 w-full'
            value={operation}
            disabled={loading || bindings.length === 0}
            onChange={(event) => setOperation(event.target.value)}
          >
            {bindings.map((binding) => (
              <NativeSelectOption
                key={binding.operation}
                value={binding.operation}
              >
                {binding.operation}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
        {selectedBinding && (
          <div className='flex flex-wrap items-center gap-2 text-xs'>
            <Badge variant='outline'>
              {selectedBinding.profile_key}@{selectedBinding.profile_version}
            </Badge>
            <Badge variant='secondary'>
              v{selectedBinding.contract_version}
            </Badge>
            <code className='text-muted-foreground'>
              {selectedBinding.contract_hash.slice(0, 12)}
            </code>
          </div>
        )}
      </div>

      {loading && (
        <div className='flex h-36 items-center justify-center rounded-lg border'>
          <Loader2 className='size-5 animate-spin' />
        </div>
      )}
      {!loading && !selectedBinding && (
        <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
          {t('This model has no operation binding yet.')}
        </div>
      )}
      {!loading && selectedBinding && (
        <>
          {props.showCapabilitySummary && (
            <ModelCapabilitySummary
              model={props.model}
              binding={selectedBinding}
            />
          )}
          <section className='rounded-lg border'>
            <header className='flex flex-wrap items-center justify-between gap-3 border-b p-4'>
              <div>
                <h3 className='font-medium'>{t('Model parameters')}</h3>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(
                    'These controls are rendered by TapLater and the playground after publication.'
                  )}
                </p>
              </div>
              <div className='flex flex-wrap gap-2'>
                <Button
                  size='sm'
                  variant='outline'
                  onClick={() =>
                    setParameters((current) => [
                      ...current,
                      {
                        clientKey: crypto.randomUUID(),
                        originalName: '',
                        name: `parameter_${current.length + 1}`,
                        label: t('New parameter'),
                        type: 'string',
                        widget: 'select',
                        placement: 'advanced',
                        enumText: '',
                        defaultText: '',
                        minimumText: '',
                        maximumText: '',
                        stepText: '',
                        required: false,
                      },
                    ])
                  }
                >
                  <Plus className='size-4' />
                  {t('Add parameter')}
                </Button>
                <Button
                  size='sm'
                  disabled={saving !== null}
                  onClick={() => void publishVersion()}
                >
                  {saving === 'publish' ? (
                    <Loader2 className='size-4 animate-spin' />
                  ) : (
                    <FileCheck2 className='size-4' />
                  )}
                  {t('Publish version')}
                </Button>
              </div>
            </header>
            <div className='px-4'>
              {parameters.length === 0 ? (
                <p className='text-muted-foreground py-8 text-center text-sm'>
                  {t('No visible parameters. Add one to begin.')}
                </p>
              ) : (
                parameters.map((parameter, index) => (
                  <ParameterRow
                    key={parameter.clientKey}
                    parameter={parameter}
                    onChange={(next) =>
                      setParameters((current) =>
                        current.map((item, itemIndex) =>
                          itemIndex === index ? next : item
                        )
                      )
                    }
                    onDelete={() => {
                      if (parameter.originalName) {
                        setDeletedParameterNames((current) => {
                          const next = new Set(current)
                          next.add(parameter.originalName)
                          return next
                        })
                      }
                      setParameters((current) =>
                        current.filter((_, itemIndex) => itemIndex !== index)
                      )
                    }}
                  />
                ))
              )}
            </div>
          </section>

          <section className='rounded-lg border'>
            <header className='flex items-center gap-2 border-b p-4'>
              <History className='size-4' />
              <div>
                <h3 className='font-medium'>{t('Binding revisions')}</h3>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(
                    'Rollback uses the current contract hash to prevent overwriting a newer edit.'
                  )}
                </p>
              </div>
            </header>
            <div className='divide-y'>
              {revisions.length === 0 ? (
                <p className='text-muted-foreground p-6 text-center text-sm'>
                  {t('No revisions yet.')}
                </p>
              ) : (
                revisions.map((revision) => (
                  <div
                    key={revision.id}
                    className='flex flex-wrap items-center justify-between gap-3 px-4 py-3'
                  >
                    <div className='min-w-0'>
                      <p className='text-sm font-medium'>
                        r{revision.revision} · {revision.profile_key}@
                        {revision.profile_version}
                      </p>
                      <p className='text-muted-foreground mt-1 font-mono text-xs'>
                        v{revision.contract_version} ·{' '}
                        {revision.contract_hash.slice(0, 12)} ·{' '}
                        {timestampLabel(revision.created_time)}
                      </p>
                    </div>
                    <Button
                      size='sm'
                      variant='outline'
                      disabled={
                        revision.contract_hash === selectedBinding.contract_hash
                      }
                      onClick={() => setRollbackRevision(revision)}
                    >
                      <RotateCcw className='size-4' />
                      {t('Rollback')}
                    </Button>
                  </div>
                ))
              )}
            </div>
          </section>

          <section className='rounded-lg border'>
            <header className='border-b p-4'>
              <h3 className='font-medium'>{t('Parameter evidence')}</h3>
              <p className='text-muted-foreground mt-1 text-xs'>
                {t(
                  'Record documentation, demo constraints, and real test results without changing the contract hash.'
                )}
              </p>
            </header>
            <div className='grid gap-4 p-4 xl:grid-cols-[minmax(0,1fr)_340px]'>
              <div className='divide-y rounded-lg border'>
                {evidence.length === 0 ? (
                  <p className='text-muted-foreground p-6 text-center text-sm'>
                    {t('No evidence recorded.')}
                  </p>
                ) : (
                  evidence.map((item) => (
                    <div key={item.id} className='flex gap-3 p-3'>
                      <div className='min-w-0 flex-1'>
                        <div className='flex flex-wrap items-center gap-2'>
                          <code className='text-sm font-medium'>
                            {item.field}
                          </code>
                          <Badge variant='outline'>{item.source_type}</Badge>
                          <Badge
                            variant={
                              item.verification_status === 'tested'
                                ? 'secondary'
                                : 'outline'
                            }
                          >
                            {item.verification_status}
                          </Badge>
                        </div>
                        <p className='text-muted-foreground mt-1 line-clamp-2 text-xs'>
                          {item.notes || item.source_locator || '-'}
                        </p>
                        {item.source_url && (
                          <a
                            className='text-primary mt-1 inline-flex items-center gap-1 text-xs hover:underline'
                            href={item.source_url}
                            target='_blank'
                            rel='noreferrer'
                          >
                            {t('Open source')}
                            <ExternalLink className='size-3' />
                          </a>
                        )}
                      </div>
                      <div className='flex shrink-0 items-start gap-1'>
                        <Button
                          size='sm'
                          variant='ghost'
                          onClick={() => editEvidence(item)}
                        >
                          {t('Edit')}
                        </Button>
                        <Button
                          size='icon-sm'
                          variant='ghost'
                          aria-label={t('Delete evidence')}
                          onClick={() => void removeEvidence(item.id)}
                        >
                          <Trash2 className='size-4' />
                        </Button>
                      </div>
                    </div>
                  ))
                )}
              </div>
              <div className='space-y-3 rounded-lg border p-3'>
                <div className='flex items-center justify-between'>
                  <h4 className='text-sm font-medium'>
                    {evidenceDraft.id ? t('Edit evidence') : t('Add evidence')}
                  </h4>
                  {evidenceDraft.id > 0 && (
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={() => setEvidenceDraft(emptyEvidence)}
                    >
                      {t('Cancel edit')}
                    </Button>
                  )}
                </div>
                <label className='space-y-1.5'>
                  <Label className='text-xs'>{t('Parameter')}</Label>
                  <NativeSelect
                    className='h-8 w-full text-xs'
                    value={evidenceDraft.field}
                    onChange={(event) =>
                      setEvidenceDraft((current) => ({
                        ...current,
                        field: event.target.value,
                      }))
                    }
                  >
                    <NativeSelectOption value=''>
                      {t('Select parameter')}
                    </NativeSelectOption>
                    {parameters.map((parameter) => (
                      <NativeSelectOption
                        key={parameter.name}
                        value={parameter.name}
                      >
                        {parameter.label} ({parameter.name})
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </label>
                <div className='grid grid-cols-2 gap-2'>
                  <label className='space-y-1.5'>
                    <Label className='text-xs'>{t('Source')}</Label>
                    <NativeSelect
                      className='h-8 w-full text-xs'
                      value={evidenceDraft.source_type}
                      onChange={(event) =>
                        setEvidenceDraft((current) => ({
                          ...current,
                          source_type: event.target
                            .value as EvidenceDraft['source_type'],
                        }))
                      }
                    >
                      {['doc', 'demo', 'manual', 'test'].map((value) => (
                        <NativeSelectOption key={value} value={value}>
                          {value}
                        </NativeSelectOption>
                      ))}
                    </NativeSelect>
                  </label>
                  <label className='space-y-1.5'>
                    <Label className='text-xs'>{t('Status')}</Label>
                    <NativeSelect
                      className='h-8 w-full text-xs'
                      value={evidenceDraft.verification_status}
                      onChange={(event) =>
                        setEvidenceDraft((current) => ({
                          ...current,
                          verification_status: event.target
                            .value as EvidenceDraft['verification_status'],
                        }))
                      }
                    >
                      {['unverified', 'documented', 'tested', 'rejected'].map(
                        (value) => (
                          <NativeSelectOption key={value} value={value}>
                            {value}
                          </NativeSelectOption>
                        )
                      )}
                    </NativeSelect>
                  </label>
                </div>
                <Input
                  className='h-8 text-xs'
                  placeholder={t('Source URL')}
                  value={evidenceDraft.source_url}
                  onChange={(event) =>
                    setEvidenceDraft((current) => ({
                      ...current,
                      source_url: event.target.value,
                    }))
                  }
                />
                <Input
                  className='h-8 text-xs'
                  placeholder={t('Source locator')}
                  value={evidenceDraft.source_locator}
                  onChange={(event) =>
                    setEvidenceDraft((current) => ({
                      ...current,
                      source_locator: event.target.value,
                    }))
                  }
                />
                <Input
                  className='h-8 text-xs'
                  type='date'
                  value={evidenceDraft.verified_at}
                  onChange={(event) =>
                    setEvidenceDraft((current) => ({
                      ...current,
                      verified_at: event.target.value,
                    }))
                  }
                />
                <Textarea
                  className='min-h-20 text-xs'
                  placeholder={t('Notes')}
                  value={evidenceDraft.notes}
                  onChange={(event) =>
                    setEvidenceDraft((current) => ({
                      ...current,
                      notes: event.target.value,
                    }))
                  }
                />
                <Button
                  className='w-full'
                  size='sm'
                  disabled={saving === 'evidence'}
                  onClick={() => void saveEvidence()}
                >
                  {saving === 'evidence' ? (
                    <Loader2 className='size-4 animate-spin' />
                  ) : (
                    <Save className='size-4' />
                  )}
                  {t('Save evidence')}
                </Button>
              </div>
            </div>
          </section>
        </>
      )}

      <ConfirmDialog
        open={rollbackRevision !== null}
        onOpenChange={(open) => {
          if (!open) setRollbackRevision(null)
        }}
        title={t('Rollback contract binding')}
        desc={t(
          'The selected historical binding becomes the active contract immediately. Continue?'
        )}
        confirmText={t('Rollback')}
        isLoading={saving === 'publish'}
        handleConfirm={() => void rollback()}
      />
    </div>
  )
}
