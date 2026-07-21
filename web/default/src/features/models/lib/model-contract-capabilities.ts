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
  Model,
  ModelContractObject,
  ModelOperationBinding,
} from '../types'

export const MATERIAL_CAPABILITY_KINDS = ['image', 'video', 'audio'] as const

export type MaterialCapabilityKind = (typeof MATERIAL_CAPABILITY_KINDS)[number]

export type ModelOutputKind =
  | 'text'
  | MaterialCapabilityKind
  | 'embedding'
  | 'ranking'
  | 'unknown'

export type MaterialInputCapability = {
  kind: MaterialCapabilityKind
  minItems: number
  maxItems: number | null
}

export type DownstreamInputRule =
  | { kind: 'prompt' }
  | { kind: 'material'; materialKind: MaterialCapabilityKind }
  | { kind: 'none' }

export type ModelContractCapabilities = {
  modelType: string | null
  prompt: {
    accepted: boolean
    required: boolean
  }
  textInput: {
    fields: string[]
    required: boolean
  }
  materials: MaterialInputCapability[]
  outputKind: ModelOutputKind
  downstreamRule: DownstreamInputRule
}

const OUTPUT_KIND_BY_OPERATION: Record<string, ModelOutputKind> = {
  'text.chat': 'text',
  'image.generate': 'image',
  'video.generate': 'video',
  'audio.generate': 'audio',
  'embedding.create': 'embedding',
  'rerank.create': 'ranking',
}

const TEXT_INPUT_FIELDS = ['input', 'query', 'documents'] as const

function objectValue(value: unknown): ModelContractObject {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return {}
  return value as ModelContractObject
}

function nonNegativeInteger(value: unknown): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0
  return Math.max(0, Math.trunc(value))
}

export function deriveModelContractCapabilities(
  model: Pick<Model, 'model_type'> | undefined,
  binding: Pick<ModelOperationBinding, 'operation' | 'effective_contract'>
): ModelContractCapabilities {
  const inputSchema = objectValue(binding.effective_contract.input_schema)
  const properties = objectValue(inputSchema.properties)
  const requiredFields = new Set(
    Array.isArray(inputSchema.required)
      ? inputSchema.required.filter(
          (field): field is string => typeof field === 'string'
        )
      : []
  )
  const acceptsPrompt = Object.hasOwn(properties, 'prompt')
  const textInputFields = TEXT_INPUT_FIELDS.filter((field) =>
    Object.hasOwn(properties, field)
  )
  const materialSchema = objectValue(binding.effective_contract.material_schema)
  const materials = MATERIAL_CAPABILITY_KINDS.flatMap((kind) => {
    if (!Object.hasOwn(materialSchema, kind)) return []
    const rule = objectValue(materialSchema[kind])
    const hasMaximum = Object.hasOwn(rule, 'max_items')
    const maxItems = hasMaximum ? nonNegativeInteger(rule.max_items) : null
    if (maxItems === 0) return []
    const minItems = nonNegativeInteger(rule.min_items)
    return [
      {
        kind,
        minItems: maxItems === null ? minItems : Math.min(minItems, maxItems),
        maxItems,
      },
    ]
  })
  const outputKind =
    OUTPUT_KIND_BY_OPERATION[binding.operation.trim().toLowerCase()] ||
    'unknown'

  let downstreamRule: DownstreamInputRule = { kind: 'none' }
  if (outputKind === 'text') {
    downstreamRule = { kind: 'prompt' }
  } else if (
    outputKind === 'image' ||
    outputKind === 'video' ||
    outputKind === 'audio'
  ) {
    downstreamRule = { kind: 'material', materialKind: outputKind }
  }

  return {
    modelType: model?.model_type?.trim() || null,
    prompt: {
      accepted: acceptsPrompt,
      required: acceptsPrompt && requiredFields.has('prompt'),
    },
    textInput: {
      fields: textInputFields,
      required: textInputFields.some((field) => requiredFields.has(field)),
    },
    materials,
    outputKind,
    downstreamRule,
  }
}
