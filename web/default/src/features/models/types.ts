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
import { z } from 'zod'

// ============================================================================
// Model Types
// ============================================================================

/**
 * Bound channel information
 */
export interface BoundChannel {
  name: string
  type: number
  channel_provider?: string
}

/**
 * Model entity from API
 */
export interface Model {
  id: number
  model_name: string
  display_name?: string
  model_type?: string
  description?: string
  source_url?: string
  icon?: string
  tags?: string
  vendor_id?: number
  endpoints?: string
  status: number
  sync_official: number
  created_time: number
  updated_time: number
  name_rule: number
  // Runtime fields
  channel_providers?: string[]
  bound_channels?: BoundChannel[]
  enable_groups?: string[]
  quota_types?: number[]
  matched_models?: string[]
  matched_count?: number
}

/**
 * Vendor entity from API
 */
export interface Vendor {
  id: number
  name: string
  description?: string
  icon?: string
  status: number
  created_time: number
  updated_time: number
}

/**
 * Prefill group entity
 */
export interface PrefillGroup {
  id: number
  name: string
  type: 'model' | 'tag' | 'endpoint'
  items: string | string[]
  description?: string
}

export type ModelContractObject = Record<string, unknown>

export interface ModelOperationProfile {
  profile_key: string
  display_name: string
  description?: string
  version: number
  operation: string
  endpoint_type: string
  execution_mode: string
  input_schema: ModelContractObject
  ui_schema: ModelContractObject
  material_schema: ModelContractObject
  response_contract: string
  smoke_test: ModelContractObject
  status: 'draft' | 'published' | 'archived'
  created_time?: number
  updated_time?: number
}

export interface ModelOperationBinding {
  model_name: string
  operation: string
  profile_key: string
  profile_version: number
  contract_version: number
  contract_hash: string
  overrides: ModelContractObject
  effective_contract?: ModelContractObject
  enabled: boolean
}

export interface ModelOperationProfilesResponse {
  success: boolean
  message?: string
  data?: {
    items: ModelOperationProfile[]
    total: number
    page: number
    page_size: number
  }
}

export interface ModelOperationBindingsResponse {
  success: boolean
  message?: string
  data?: ModelOperationBinding[]
}

export interface ModelOperationRequestContract {
  adapter?: string
  field_map?: Record<string, string>
  coercions?: Record<string, string>
}

export interface ModelOperationEffectiveContract extends ModelContractObject {
  contract_version?: number
  contract_hash?: string
  response_contract?: string
  request_contract?: ModelOperationRequestContract
  dispatch_path?: string
}

export interface ModelTokenProfileData {
  model_id: string
  dispatch_ready: boolean
  profile: ModelOperationProfile
  binding: ModelOperationBinding
  effective_contract: ModelOperationEffectiveContract
}

export interface ModelTokenProfileResponse {
  success: boolean
  message?: string
  data?: ModelTokenProfileData
}

export interface ModelTokenQuoteData {
  model_id: string
  operation: string
  effective_group: string
  billing_mode: string
  pricing_version: string
  contract_version: number
  contract_hash: string
  base_price: number
  model_ratio: number
  completion_ratio: number
  group_ratio: number
  parameter_multipliers?: Record<string, number> | null
  estimated_quota: number
  estimated_amount: number
  estimate_kind: string
}

export interface ModelTokenQuoteResponse {
  success: boolean
  message?: string
  data?: ModelTokenQuoteData
}

// ============================================================================
// API Request/Response Types
// ============================================================================

/**
 * Get models list parameters
 */
export interface GetModelsParams {
  p?: number
  page_size?: number
  vendor?: string // vendor ID to filter by
  channel_provider?: string // API channel provider code
  status?: string // filter by status
  sync_official?: string // filter by sync_official status
}

/**
 * Search models parameters
 */
export interface SearchModelsParams {
  keyword?: string
  vendor?: string // vendor ID to filter by
  channel_provider?: string // API channel provider code
  status?: string // filter by status
  sync_official?: string // filter by sync_official status
  p?: number
  page_size?: number
}

/**
 * Get models response
 */
export interface GetModelsResponse {
  success: boolean
  message?: string
  data?: {
    items: Model[]
    total: number
    page: number
    page_size: number
    vendor_counts?: Record<string, number>
    channel_provider_counts?: Record<string, number>
  }
}

/**
 * Get model detail response
 */
export interface GetModelResponse {
  success: boolean
  message?: string
  data?: Model
}

/**
 * Get vendors response
 */
export interface GetVendorsResponse {
  success: boolean
  message?: string
  data?: {
    items: Vendor[]
    total: number
    page: number
    page_size: number
  }
}

/**
 * Get vendor response
 */
export interface GetVendorResponse {
  success: boolean
  message?: string
  data?: Vendor
}

/**
 * Sync diff data
 */
export interface SyncDiffData {
  missing?: Array<{
    model_name: string
    vendor?: string
    [key: string]: unknown
  }>
  conflicts?: Array<{
    model_name: string
    local?: Partial<Model>
    upstream?: Partial<Model>
    fields?: Array<{
      field: string
      local?: unknown
      upstream?: unknown
    }>
    [key: string]: unknown
  }>
}

export interface SyncOverwritePayload {
  model_name: string
  fields: string[]
}

/**
 * Sync upstream response
 */
export interface SyncUpstreamResponse {
  success: boolean
  message?: string
  data?: {
    created_models?: number
    updated_models?: number
    created_vendors?: number
    skipped_models?: string[]
  }
}

/**
 * Preview upstream diff response
 */
export interface PreviewUpstreamDiffResponse {
  success: boolean
  message?: string
  data?: SyncDiffData
}

/**
 * Missing models response
 */
export interface MissingModelsResponse {
  success: boolean
  message?: string
  data?: string[]
}

/**
 * Prefill groups response
 */
export interface PrefillGroupsResponse {
  success: boolean
  message?: string
  data?: PrefillGroup[]
}

// ============================================================================
// Form Data Types
// ============================================================================

/**
 * Model form schema
 */
export const modelFormSchema = z.object({
  id: z.number().optional(),
  model_name: z.string().min(1, 'Model name is required'),
  display_name: z.string().default(''),
  model_type: z.string().default(''),
  description: z.string().default(''),
  source_url: z.union([z.literal(''), z.string().url()]).default(''),
  icon: z.string().default(''),
  tags: z.array(z.string()).default([]),
  vendor_id: z.number().optional(),
  endpoints: z.string().default(''),
  name_rule: z.number().min(0).max(3).default(0),
  status: z.boolean().default(true),
  sync_official: z.boolean().default(true),
})

export type ModelFormValues = z.infer<typeof modelFormSchema>

/**
 * Vendor form schema
 */
export const vendorFormSchema = z.object({
  id: z.number().optional(),
  name: z.string().min(1, 'Vendor name is required'),
  description: z.string().default(''),
  icon: z.string().default(''),
  status: z.number().default(1),
})

export type VendorFormValues = z.infer<typeof vendorFormSchema>

/**
 * Prefill group form schema
 */
export const prefillGroupFormSchema = z.object({
  id: z.number().optional(),
  name: z.string().min(1, 'Group name is required'),
  description: z.string().optional(),
  type: z.enum(['model', 'tag', 'endpoint']),
  items: z.union([z.string(), z.array(z.string())]),
})

export type PrefillGroupFormValues = z.infer<typeof prefillGroupFormSchema>

const jsonObjectString = z.string().refine((value) => {
  try {
    const parsed = JSON.parse(value || '{}')
    return (
      parsed !== null && typeof parsed === 'object' && !Array.isArray(parsed)
    )
  } catch {
    return false
  }
}, 'Must be a JSON object')

export const modelOperationProfileFormSchema = z.object({
  profile_key: z.string().min(1, 'Profile key is required'),
  display_name: z.string().min(1, 'Display name is required'),
  description: z.string(),
  version: z.number().int().positive(),
  operation: z.string().min(1, 'Operation is required'),
  endpoint_type: z.string().min(1, 'Endpoint type is required'),
  execution_mode: z.enum(['sync', 'async']),
  response_contract: z.string().min(1, 'Response contract is required'),
  status: z.enum(['draft', 'published', 'archived']),
  input_schema: jsonObjectString,
  ui_schema: jsonObjectString,
  material_schema: jsonObjectString,
  smoke_test: jsonObjectString,
})

export type ModelOperationProfileFormValues = z.infer<
  typeof modelOperationProfileFormSchema
>

export const modelOperationBindingFormSchema = z.object({
  model_name: z.string().min(1, 'Model name is required'),
  operation: z.string().min(1, 'Operation is required'),
  profile_key: z.string().min(1, 'Profile key is required'),
  profile_version: z.number().int().positive(),
  overrides: jsonObjectString,
  enabled: z.boolean(),
})

export type ModelOperationBindingFormValues = z.infer<
  typeof modelOperationBindingFormSchema
>

// ============================================================================
// Utility Types
// ============================================================================

/**
 * Name rule type
 */
export type NameRule = 0 | 1 | 2 | 3 // exact, prefix, contains, suffix

/**
 * Model status type
 */
export type ModelStatus = 0 | 1 // disabled, enabled

/**
 * Quota type
 */
export type QuotaType = 0 | 1 // usage-based, per-call

/**
 * Sync locale
 */
export type SyncLocale = 'zh' | 'en' | 'ja'

/**
 * Sync upstream source
 */
export type SyncSource = 'official' | 'config'

// ============================================================================
// Model Deployments Types
// ============================================================================

/**
 * Model tab type
 */
export type ModelTabCategory = 'metadata' | 'deployments'

/**
 * Deployment entity from API
 */
export interface Deployment {
  id: string | number
  container_name?: string
  deployment_name?: string
  name?: string
  status?: string
  provider?: string
  /**
   * Human readable string returned by backend, e.g. "2 hour 15 minutes"
   * or "completed".
   */
  time_remaining?: string
  /**
   * Remaining minutes (numeric) returned by backend.
   */
  compute_minutes_remaining?: number
  /**
   * Served minutes (numeric) returned by backend.
   */
  compute_minutes_served?: number
  /**
   * Completed percent (0-100) returned by backend.
   */
  completed_percent?: number
  hardware_info?: string | Record<string, unknown>
  hardware_name?: string
  brand_name?: string
  hardware_quantity?: number
  created_at?: string | number
  updated_at?: string | number
  [key: string]: unknown
}

/**
 * Deployment settings response
 */
export interface DeploymentSettingsResponse {
  success: boolean
  message?: string
  data?: {
    enabled?: boolean
    [key: string]: unknown
  }
}

/**
 * List deployments response
 */
export interface ListDeploymentsResponse {
  success: boolean
  message?: string
  data?: {
    items?: Deployment[]
    total?: number
    page?: number
    page_size?: number
    status_counts?: Record<string, number>
  }
}

/**
 * Deployment logs response
 */
export interface DeploymentLogsResponse {
  success: boolean
  message?: string
  data?: {
    logs?: Array<{
      timestamp?: string
      level?: string
      message?: string
      source?: string
    }>
    cursor?: string
  }
}
