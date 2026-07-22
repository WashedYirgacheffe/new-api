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
export interface Subsite {
  id: number
  code: string
  name: string
  domain: string
  version: string
  route_group: string
  enabled: boolean
  created_time?: number
  updated_time?: number
}

export interface ClaimableSubsite extends Subsite {
  claimed: boolean
}

export interface SubsiteCatalogModel {
  model_id: string
  display_name: string
  description?: string
  brand_icon?: string
  model_type: string
  model_provider: string
  channel_providers: string[]
  supported_endpoint_types: string[]
  billing_mode: string
  base_price?: number
  model_ratio?: number
  completion_ratio?: number
  pricing_version: string
  profile_ready: boolean
  routing_groups: string[]
  routable: boolean
  price_ready: boolean
  enabled: boolean
}

export interface SubsiteModelsData {
  site: Subsite
  items: SubsiteCatalogModel[]
  enabled_model_ids: string[]
}

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface SubsiteFormPayload {
  id?: number
  code: string
  name: string
  domain: string
  version: string
  route_group: string
  claim_password?: string
  enabled: boolean
}
