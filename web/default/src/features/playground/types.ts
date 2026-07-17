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
// Message types
export type MessageRole = 'user' | 'assistant' | 'system'

export type MessageStatus = 'loading' | 'streaming' | 'complete' | 'error'

export type PlaygroundMessageLayoutMode = 'alternating' | 'left'

export interface MessageVersion {
  id: string
  content: string
}

export interface Message {
  key: string
  from: MessageRole
  versions: MessageVersion[]
  createdAt?: number
  startedAt?: number
  completedAt?: number
  durationMs?: number
  sources?: { href: string; title: string }[]
  reasoning?: {
    content: string
    duration: number
    startedAt?: number
    completedAt?: number
    durationMs?: number
  }
  isReasoningStreaming?: boolean
  isReasoningComplete?: boolean
  isContentComplete?: boolean
  status?: MessageStatus
  errorCode?: string | null
}

// API payload types
export interface ChatCompletionMessage {
  role: MessageRole
  content: string | ContentPart[]
}

export interface ContentPart {
  type: 'text' | 'image_url'
  text?: string
  image_url?: {
    url: string
  }
}

export interface ChatCompletionRequest {
  model: string
  group?: string
  messages: ChatCompletionMessage[]
  stream: boolean
  temperature?: number
  top_p?: number
  max_tokens?: number
  frequency_penalty?: number
  presence_penalty?: number
  seed?: number
}

export interface ChatCompletionChunk {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    delta: {
      role?: MessageRole
      content?: string
      reasoning_content?: string
    }
    finish_reason: string | null
  }>
}

export interface ChatCompletionResponse {
  id: string
  object: string
  created: number
  model: string
  choices: Array<{
    index: number
    message: {
      role: MessageRole
      content: string
      reasoning_content?: string
    }
    finish_reason: string
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

// Configuration types
export interface PlaygroundConfig {
  model: string
  group: string
  temperature: number
  top_p: number
  max_tokens: number
  frequency_penalty: number
  presence_penalty: number
  seed: number | null
  stream: boolean
}

export interface ParameterEnabled {
  temperature: boolean
  top_p: boolean
  max_tokens: boolean
  frequency_penalty: boolean
  presence_penalty: boolean
  seed: boolean
}

// Model and group options
export interface ModelOption {
  label: string
  value: string
}

export interface GroupOption {
  label: string
  value: string
  ratio: number
  desc?: string
}

export type PlaygroundOperation =
  | 'text.chat'
  | 'image.generate'
  | 'video.generate'

export type ContractObject = Record<string, unknown>

export interface PlaygroundEffectiveContract extends ContractObject {
  operation?: PlaygroundOperation
  endpoint_type?: string
  execution_mode?: 'sync' | 'async'
  input_schema?: ContractObject
  ui_schema?: ContractObject
  material_schema?: ContractObject
  request_contract?: {
    adapter?: string
    field_map?: Record<string, string>
    coercions?: Record<string, string>
  }
  parameter_defaults?: ContractObject
  parameter_overrides?: ContractObject
  dispatch_path?: string
  response_contract?: string
  contract_version?: number
  contract_hash?: string
}

export interface PlaygroundCatalogBinding {
  operation: PlaygroundOperation
  profile_key: string
  profile_version: number
  contract_version: number
  contract_hash: string
  endpoint_type: string
  execution_mode: 'sync' | 'async'
  response_contract: string
  effective_contract?: PlaygroundEffectiveContract
  dispatch_ready: boolean
}

export interface PlaygroundCatalogModel {
  model_id: string
  display_name: string
  description?: string
  brand_icon?: string
  model_type: string
  billing_mode: string
  base_price?: number
  pricing_version: string
  profile_bindings: PlaygroundCatalogBinding[]
  routing_groups: string[]
  routable: boolean
  price_ready: boolean
  profile_ready: boolean
}

export interface PlaygroundCatalogData {
  items: PlaygroundCatalogModel[]
  total: number
  token_group: string
  pricing_version: string
}

export interface PlaygroundCatalogResponse {
  success: boolean
  message?: string
  data?: PlaygroundCatalogData
}

export interface PlaygroundQuoteData {
  model_id: string
  operation: PlaygroundOperation
  effective_group: string
  pricing_version: string
  contract_version: number
  contract_hash: string
  estimated_quota: number
  estimated_amount: number
  estimate_kind: string
  parameter_multipliers?: Record<string, number> | null
}

export interface PlaygroundQuoteResponse {
  success: boolean
  message?: string
  data?: PlaygroundQuoteData
}

export interface PlaygroundMaterials {
  image: PlaygroundMaterialItem[]
  video: PlaygroundMaterialItem[]
  audio: PlaygroundMaterialItem[]
}

export interface PlaygroundMaterialItem {
  id: string
  source: string
  mime_type: string
  size_mb?: number
  duration_seconds?: number
}
