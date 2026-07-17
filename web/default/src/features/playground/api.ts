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

import { API_ENDPOINTS } from './constants'
import type {
  ChatCompletionRequest,
  ChatCompletionResponse,
  ContractObject,
  ModelOption,
  GroupOption,
  PlaygroundCatalogResponse,
  PlaygroundOperation,
  PlaygroundQuoteResponse,
} from './types'

/**
 * Send chat completion request (non-streaming)
 */
export async function sendChatCompletion(
  payload: ChatCompletionRequest,
  signal?: AbortSignal
): Promise<ChatCompletionResponse> {
  const res = await api.post(API_ENDPOINTS.CHAT_COMPLETIONS, payload, {
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Get user available models
 */
export async function getUserModels(group: string): Promise<ModelOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_MODELS, {
    params: { group },
  })
  const { data } = res

  if (!data.success || !Array.isArray(data.data)) {
    return []
  }

  return data.data.map((model: string) => ({
    label: model,
    value: model,
  }))
}

/**
 * Get user groups
 */
export async function getUserGroups(): Promise<GroupOption[]> {
  const res = await api.get(API_ENDPOINTS.USER_GROUPS)
  const { data } = res

  if (!data.success || !data.data) {
    return []
  }

  const groupData = data.data as Record<string, { desc: string; ratio: number }>

  // label is for button display (name only); desc is for dropdown content
  return Object.entries(groupData).map(([group, info]) => ({
    label: group,
    value: group,
    ratio: info.ratio,
    desc: info.desc,
  }))
}

export async function getPlaygroundCatalog(
  group: string
): Promise<PlaygroundCatalogResponse> {
  const res = await api.get(API_ENDPOINTS.MODEL_CATALOG, {
    params: { group },
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function quotePlaygroundModel(
  group: string,
  model: string,
  operation: PlaygroundOperation,
  parameters: ContractObject,
  signal?: AbortSignal
): Promise<PlaygroundQuoteResponse> {
  const res = await api.post(
    API_ENDPOINTS.MODEL_QUOTE,
    { model, operation, parameters },
    {
      params: { group },
      headers: { 'X-CarLab-Operation': operation },
      signal,
      skipErrorHandler: true,
    } as Record<string, unknown>
  )
  return res.data
}

export async function runPlaygroundMedia(
  path: string,
  group: string,
  operation: PlaygroundOperation,
  body: ContractObject,
  signal?: AbortSignal
): Promise<unknown> {
  const res = await api.post(path, body, {
    params: { group },
    headers: { 'X-CarLab-Operation': operation },
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}

export async function getPlaygroundVideo(
  taskId: string,
  group: string,
  operation: PlaygroundOperation,
  signal?: AbortSignal
): Promise<unknown> {
  // The server normalizes every supported upstream poll path through this route.
  const res = await api.get(`/pg/videos/${encodeURIComponent(taskId)}`, {
    params: { group },
    headers: { 'X-CarLab-Operation': operation },
    signal,
    skipErrorHandler: true,
  } as Record<string, unknown>)
  return res.data
}
