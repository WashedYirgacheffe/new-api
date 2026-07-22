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
import { api } from '@/lib/api'

import type {
  ApiResponse,
  ClaimableSubsite,
  Subsite,
  SubsiteFormPayload,
  SubsiteModelsData,
} from './types'

export async function getClaimableSubsites(): Promise<
  ApiResponse<{ items: ClaimableSubsite[] }>
> {
  const res = await api.get('/api/subsites/claimable')
  return res.data
}

export async function claimSubsite(
  code: string,
  password: string
): Promise<ApiResponse<Subsite>> {
  const res = await api.post('/api/subsites/claim', { code, password })
  return res.data
}

export async function getSubsiteModels(
  code: string
): Promise<ApiResponse<SubsiteModelsData>> {
  const res = await api.get(`/api/subsites/${encodeURIComponent(code)}/models`)
  return res.data
}

export async function updateSubsiteModels(
  code: string,
  modelIds: string[]
): Promise<ApiResponse<SubsiteModelsData>> {
  const res = await api.put(
    `/api/subsites/${encodeURIComponent(code)}/models`,
    { model_ids: modelIds }
  )
  return res.data
}

export async function getSubsites(): Promise<
  ApiResponse<{ items: Subsite[] }>
> {
  const res = await api.get('/api/subsites')
  return res.data
}

export async function createSubsite(
  payload: SubsiteFormPayload
): Promise<ApiResponse<Subsite>> {
  const res = await api.post('/api/subsites', payload)
  return res.data
}

export async function updateSubsite(
  code: string,
  payload: SubsiteFormPayload
): Promise<ApiResponse<Subsite>> {
  const res = await api.put(
    `/api/subsites/${encodeURIComponent(code)}`,
    payload
  )
  return res.data
}

export async function deleteSubsite(code: string): Promise<ApiResponse> {
  const res = await api.delete(`/api/subsites/${encodeURIComponent(code)}`)
  return res.data
}
