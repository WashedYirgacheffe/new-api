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
import type {
  ContractObject,
  PlaygroundCatalogBinding,
  PlaygroundEffectiveContract,
  PlaygroundMaterials,
} from '../types'

export type ParameterType = 'string' | 'number' | 'integer' | 'boolean'

export type ParameterDescriptor = {
  name: string
  label: string
  type: ParameterType
  required: boolean
  placement: string
  widget: string
  enumValues: Array<string | number | boolean>
  minimum?: number
  maximum?: number
  step?: number
  defaultValue?: unknown
}

export type MediaOutput = {
  source: string
  mimeType: string
}

export type VideoTaskState = {
  taskId: string
  status: string
  progress?: number
  source?: string
  error?: string
}

export type MaterialKind = 'image' | 'video' | 'audio'

export type MaterialRule = {
  kind: MaterialKind
  minItems: number
  maxItems: number
  mimeTypes: string[]
  maxSizeMb?: number
  maxTotalDuration?: number
  requestField: string
  transport: string
}

export type MaterialValidationIssue = {
  key: string
  values?: Record<string, number | string>
}

const MATERIAL_KINDS: MaterialKind[] = ['image', 'video', 'audio']

function objectValue(value: unknown): ContractObject | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  return value as ContractObject
}

function stringMap(value: unknown): Record<string, string> {
  const object = objectValue(value)
  if (!object) return {}
  const result: Record<string, string> = {}
  for (const [key, item] of Object.entries(object)) {
    if (typeof item === 'string') result[key] = item
  }
  return result
}

function primitiveEnum(value: unknown): Array<string | number | boolean> {
  if (!Array.isArray(value)) return []
  return value.filter(
    (item): item is string | number | boolean =>
      typeof item === 'string' ||
      typeof item === 'number' ||
      typeof item === 'boolean'
  )
}

function finiteNumber(value: unknown): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) return undefined
  return value
}

function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter((item): item is string => typeof item === 'string')
}

export function buildMaterialRules(
  contract: PlaygroundEffectiveContract | undefined
): Record<MaterialKind, MaterialRule> {
  const schema = objectValue(contract?.material_schema) || {}
  const result = {} as Record<MaterialKind, MaterialRule>
  for (const kind of MATERIAL_KINDS) {
    const rule = objectValue(schema[kind]) || {}
    result[kind] = {
      kind,
      minItems: Math.max(0, Math.trunc(finiteNumber(rule.min_items) || 0)),
      maxItems: Math.max(0, Math.trunc(finiteNumber(rule.max_items) || 0)),
      mimeTypes: stringArray(rule.mime_types),
      maxSizeMb: finiteNumber(rule.max_size_mb),
      maxTotalDuration: finiteNumber(rule.max_total_duration),
      requestField:
        typeof rule.request_field === 'string' ? rule.request_field.trim() : '',
      transport:
        typeof rule.transport === 'string' ? rule.transport.trim() : '',
    }
  }
  return result
}

function validHttpUrl(value: string): boolean {
  try {
    const url = new URL(value)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

export function validateMaterials(
  rules: Record<MaterialKind, MaterialRule>,
  materials: PlaygroundMaterials
): MaterialValidationIssue[] {
  const issues: MaterialValidationIssue[] = []
  for (const kind of MATERIAL_KINDS) {
    const rule = rules[kind]
    const items = materials[kind]
    if (rule.maxItems > 0 && !rule.requestField) {
      issues.push({
        key: 'Material contract for {{kind}} is missing request_field.',
        values: { kind },
      })
      continue
    }
    if (rule.maxItems > 0 && rule.transport !== 'url') {
      issues.push({
        key: 'Material transport {{transport}} is not supported in the playground.',
        values: { transport: rule.transport || '-' },
      })
      continue
    }
    if (items.length < rule.minItems) {
      issues.push({
        key: 'At least {{count}} {{kind}} material(s) are required.',
        values: { count: rule.minItems, kind },
      })
    }
    if (items.length > rule.maxItems) {
      issues.push({
        key: 'No more than {{count}} {{kind}} material(s) are allowed.',
        values: { count: rule.maxItems, kind },
      })
    }
    for (const item of items) {
      if (!validHttpUrl(item.source.trim())) {
        issues.push({
          key: 'Enter a valid HTTP(S) URL for every {{kind}} material.',
          values: { kind },
        })
        break
      }
    }
  }
  return issues
}

function widgetType(value: unknown): string {
  if (typeof value === 'string') return value
  const object = objectValue(value)
  return typeof object?.type === 'string' ? object.type : ''
}

function supportedParameterType(value: unknown): ParameterType | null {
  if (
    value === 'string' ||
    value === 'number' ||
    value === 'integer' ||
    value === 'boolean'
  ) {
    return value
  }
  return null
}

export function buildParameterDescriptors(
  contract: PlaygroundEffectiveContract | undefined
): ParameterDescriptor[] {
  const inputSchema = objectValue(contract?.input_schema)
  const properties = objectValue(inputSchema?.properties) || {}
  const uiSchema = objectValue(contract?.ui_schema)
  const placements = stringMap(uiSchema?.placements)
  const labels = stringMap(uiSchema?.labels)
  const widgets = objectValue(uiSchema?.widgets) || {}
  const configuredOrder = Array.isArray(uiSchema?.order)
    ? uiSchema.order.filter((item): item is string => typeof item === 'string')
    : []
  const required = new Set(
    Array.isArray(inputSchema?.required)
      ? inputSchema.required.filter(
          (item): item is string => typeof item === 'string'
        )
      : []
  )
  const order = [
    ...configuredOrder,
    ...Object.keys(properties).filter(
      (field) => !configuredOrder.includes(field)
    ),
  ]

  return order.flatMap((name) => {
    const schema = objectValue(properties[name])
    const type = supportedParameterType(schema?.type)
    const placement = placements[name] || 'advanced'
    if (
      !schema ||
      !type ||
      name === 'prompt' ||
      placement === 'prompt' ||
      placement === 'hidden' ||
      placement === 'material'
    ) {
      return []
    }
    const descriptor: ParameterDescriptor = {
      name,
      label: labels[name] || name,
      type,
      required: required.has(name),
      placement,
      widget: widgetType(widgets[name]),
      enumValues: primitiveEnum(schema.enum),
      defaultValue: schema.default,
    }
    if (typeof schema.minimum === 'number') descriptor.minimum = schema.minimum
    if (typeof schema.maximum === 'number') descriptor.maximum = schema.maximum
    if (typeof schema.multipleOf === 'number') {
      descriptor.step = schema.multipleOf
    }
    if (typeof schema.step === 'number') descriptor.step = schema.step
    return [descriptor]
  })
}

export function buildInitialParameters(
  contract: PlaygroundEffectiveContract | undefined
): ContractObject {
  const parameters: ContractObject = {}
  for (const descriptor of buildParameterDescriptors(contract)) {
    if (descriptor.defaultValue !== undefined) {
      parameters[descriptor.name] = descriptor.defaultValue
    }
  }
  const defaults = objectValue(contract?.parameter_defaults)
  if (defaults) Object.assign(parameters, defaults)
  const forced = objectValue(contract?.parameter_overrides)
  if (forced) Object.assign(parameters, forced)
  return parameters
}

export function coerceParameterValue(
  descriptor: ParameterDescriptor,
  value: string | number | boolean
): string | number | boolean {
  if (descriptor.type === 'boolean') {
    return value === true || value === 'true'
  }
  if (descriptor.type === 'integer') {
    const number = typeof value === 'number' ? value : Number(value)
    return Number.isFinite(number) ? Math.trunc(number) : 0
  }
  if (descriptor.type === 'number') {
    const number = typeof value === 'number' ? value : Number(value)
    return Number.isFinite(number) ? number : 0
  }
  return String(value)
}

function coerceMappedValue(
  value: unknown,
  coercion: string | undefined
): unknown {
  switch (coercion) {
    case 'string':
      return String(value)
    case 'integer': {
      const number = Number(value)
      return Number.isFinite(number) ? Math.trunc(number) : value
    }
    case 'number': {
      const number = Number(value)
      return Number.isFinite(number) ? number : value
    }
    case 'boolean':
      return value === true || value === 'true'
    default:
      return value
  }
}

export function mapContractParameters(
  contract: PlaygroundEffectiveContract,
  parameters: ContractObject
): ContractObject {
  const fieldMap = contract.request_contract?.field_map || {}
  const coercions = contract.request_contract?.coercions || {}
  const mapped: ContractObject = {}
  for (const [field, value] of Object.entries(parameters)) {
    mapped[fieldMap[field] || field] = coerceMappedValue(
      value,
      coercions[field]
    )
  }
  return mapped
}

function materialParts(materials: PlaygroundMaterials): ContractObject[] {
  return materials.image.map((item) => {
    return {
      fileData: {
        mimeType: item.mime_type || 'image/*',
        fileUri: item.source.trim(),
      },
    }
  })
}

function applyMaterials(
  body: ContractObject,
  contract: PlaygroundEffectiveContract,
  materials: PlaygroundMaterials
) {
  const rules = buildMaterialRules(contract)
  for (const kind of MATERIAL_KINDS) {
    const values = materials[kind]
    if (values.length === 0) continue
    const rule = rules[kind]
    if (!rule.requestField || rule.transport !== 'url') {
      throw new Error('The material contract is not dispatch-ready.')
    }
    const sources = values.map((item) => item.source.trim())
    body[rule.requestField] = sources.length === 1 ? sources[0] : sources
  }
}

export function playgroundDispatchPath(
  binding: PlaygroundCatalogBinding,
  modelId: string
): string {
  const configured = binding.effective_contract?.dispatch_path || ''
  const path = configured.replace('{model}', modelId)
  if (path.startsWith('/v1beta/models/')) return `/pg${path}`
  if (path === '/v1/chat/completions') return '/pg/chat/completions'
  if (path === '/v1/images/generations') return '/pg/images/generations'
  if (path === '/v1/videos') return '/pg/videos'
  const adapter = binding.effective_contract?.request_contract?.adapter || ''
  if (binding.operation === 'image.generate' && adapter === 'openai-chat') {
    return '/pg/chat/completions'
  }
  if (binding.operation === 'image.generate' && adapter === 'openai-image') {
    return '/pg/images/generations'
  }
  throw new Error('The model contract has no supported dispatch path.')
}

export function isPlaygroundBindingDispatchReady(
  binding: PlaygroundCatalogBinding,
  modelId: string
): boolean {
  try {
    playgroundDispatchPath(binding, modelId)
    return true
  } catch {
    return false
  }
}

export function buildQuoteParameters(
  prompt: string,
  parameters: ContractObject
): ContractObject {
  const normalizedPrompt = prompt.trim()
  if (!normalizedPrompt) throw new Error('Enter a prompt first.')
  return { ...parameters, prompt: normalizedPrompt }
}

export function missingRequiredParameter(
  descriptors: ParameterDescriptor[],
  parameters: ContractObject
): string | null {
  for (const descriptor of descriptors) {
    if (!descriptor.required) continue
    const value = parameters[descriptor.name]
    if (
      value === undefined ||
      value === null ||
      (typeof value === 'string' && value.trim() === '')
    ) {
      return descriptor.label || descriptor.name
    }
  }
  return null
}

export function buildMediaRequest(
  modelId: string,
  binding: PlaygroundCatalogBinding,
  prompt: string,
  parameters: ContractObject,
  materials: PlaygroundMaterials
): ContractObject {
  const contract = binding.effective_contract || {}
  const mapped = mapContractParameters(contract, parameters)
  const adapter = contract.request_contract?.adapter || ''
  if (adapter === 'gemini-image') {
    return {
      contents: [
        {
          role: 'user',
          parts: [{ text: prompt }, ...materialParts(materials)],
        },
      ],
      generationConfig: {
        responseModalities: ['IMAGE', 'TEXT'],
        imageConfig: {
          aspectRatio: mapped.aspectRatio || mapped.aspect_ratio,
          imageSize: mapped.imageSize || mapped.resolution,
        },
      },
    }
  }

  if (adapter === 'openai-chat') {
    return {
      model: modelId,
      messages: [{ role: 'user', content: prompt }],
      ...mapped,
    }
  }

  const body: ContractObject = {
    model: modelId,
    prompt,
    ...mapped,
  }
  applyMaterials(body, contract, materials)
  return body
}

function addOutput(
  outputs: MediaOutput[],
  seen: Set<string>,
  source: unknown,
  mimeType: string
) {
  if (typeof source !== 'string' || source.length === 0 || seen.has(source)) {
    return
  }
  seen.add(source)
  outputs.push({ source, mimeType })
}

function addTextImageOutputs(
  outputs: MediaOutput[],
  seen: Set<string>,
  text: string
) {
  for (const match of text.matchAll(
    /data:(image\/[a-z0-9.+-]+);base64,([a-z0-9+/=\s]+)/gi
  )) {
    addOutput(outputs, seen, match[0].replaceAll(/\s/g, ''), match[1])
  }
  for (const match of text.matchAll(/!\[[^\]]*\]\((https?:\/\/[^)]+)\)/g)) {
    addOutput(outputs, seen, match[1], 'image')
  }
}

export function extractImageOutputs(payload: unknown): MediaOutput[] {
  const root = objectValue(payload)
  const outputs: MediaOutput[] = []
  const seen = new Set<string>()
  const data = Array.isArray(root?.data) ? root.data : []
  for (const item of data) {
    const image = objectValue(item)
    addOutput(outputs, seen, image?.url, 'image')
    if (typeof image?.b64_json === 'string') {
      addOutput(
        outputs,
        seen,
        `data:image/png;base64,${image.b64_json}`,
        'image/png'
      )
    }
  }

  const candidates = Array.isArray(root?.candidates) ? root.candidates : []
  for (const item of candidates) {
    const candidate = objectValue(item)
    const content = objectValue(candidate?.content)
    const parts = Array.isArray(content?.parts) ? content.parts : []
    for (const rawPart of parts) {
      const part = objectValue(rawPart)
      const inlineData =
        objectValue(part?.inlineData) || objectValue(part?.inline_data)
      const inlineValue = inlineData?.data
      if (typeof inlineValue === 'string') {
        let mimeType = 'image/png'
        if (typeof inlineData?.mimeType === 'string') {
          mimeType = inlineData.mimeType
        } else if (typeof inlineData?.mime_type === 'string') {
          mimeType = inlineData.mime_type
        }
        addOutput(
          outputs,
          seen,
          `data:${mimeType};base64,${inlineValue}`,
          mimeType
        )
      }
      const fileData =
        objectValue(part?.fileData) || objectValue(part?.file_data)
      addOutput(outputs, seen, fileData?.fileUri || fileData?.file_uri, 'image')
      if (typeof part?.text === 'string') {
        addTextImageOutputs(outputs, seen, part.text)
      }
    }
  }

  const choices = Array.isArray(root?.choices) ? root.choices : []
  for (const item of choices) {
    const choice = objectValue(item)
    const message = objectValue(choice?.message)
    const content = message?.content
    if (typeof content === 'string') {
      addTextImageOutputs(outputs, seen, content)
      continue
    }
    if (!Array.isArray(content)) continue
    for (const rawPart of content) {
      const part = objectValue(rawPart)
      if (typeof part?.text === 'string') {
        addTextImageOutputs(outputs, seen, part.text)
      }
      const imageURL = objectValue(part?.image_url)
      addOutput(outputs, seen, imageURL?.url || part?.image_url, 'image')
    }
  }
  return outputs
}

export function extractVideoTask(payload: unknown): VideoTaskState {
  const root = objectValue(payload)
  const nested = objectValue(root?.data)
  const value = nested || root || {}
  const output = objectValue(value.output)
  const errorValue = objectValue(value.error)
  const taskId = String(value.task_id || value.taskId || value.id || '')
  const status = String(value.status || value.state || '')
  const rawProgress = value.progress
  let progress: number | undefined
  if (typeof rawProgress === 'number') {
    progress = rawProgress
  } else if (typeof rawProgress === 'string' && rawProgress !== '') {
    progress = Number(rawProgress)
  }
  const source =
    value.video_url || value.url || output?.video_url || output?.url
  let error: string | undefined
  if (typeof value.error === 'string') {
    error = value.error
  } else if (typeof errorValue?.message === 'string') {
    error = errorValue.message
  } else if (typeof value.fail_reason === 'string') {
    error = value.fail_reason
  } else if (typeof value.failReason === 'string') {
    error = value.failReason
  }
  return {
    taskId,
    status,
    progress: Number.isFinite(progress) ? progress : undefined,
    source: typeof source === 'string' ? source : undefined,
    error,
  }
}
