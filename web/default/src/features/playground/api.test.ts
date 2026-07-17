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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { AxiosAdapter, InternalAxiosRequestConfig } from 'axios'

import { api } from '@/lib/api'

import {
  createPlaygroundGeneration,
  deletePlaygroundGeneration,
  getPlaygroundGenerations,
  getPlaygroundVideo,
  persistableMediaSources,
  quotePlaygroundModel,
  runPlaygroundMedia,
  updatePlaygroundGeneration,
  uploadPlaygroundGenerationAsset,
} from './api'
import type { PlaygroundGeneration } from './types'

describe('media playground operation dispatch', () => {
  test('sends the selected operation on quote, generation, and polling requests', async () => {
    const requests: InternalAxiosRequestConfig[] = []
    const originalAdapter = api.defaults.adapter
    const adapter: AxiosAdapter = async (config) => {
      requests.push(config)
      return {
        data: {},
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    api.defaults.adapter = adapter

    try {
      await quotePlaygroundModel(
        'default',
        'deepwl/gpt-image-2',
        'image.generate',
        { quality: 'high' }
      )
      await runPlaygroundMedia(
        '/pg/chat/completions',
        'default',
        'image.generate',
        { model: 'deepwl/gpt-image-2' },
        'playground-generation-image'
      )
      await runPlaygroundMedia(
        '/pg/videos',
        'default',
        'video.generate',
        {
          model: 'deepwl/omni-fast-v2v',
        },
        'playground-generation-video'
      )
      await getPlaygroundVideo('task/with spaces', 'default', 'video.generate')
    } finally {
      api.defaults.adapter = originalAdapter
    }

    assert.deepEqual(
      requests.map((request) => ({
        idempotencyKey: request.headers.get('Idempotency-Key'),
        method: request.method,
        operation: request.headers.get('X-CarLab-Operation'),
        url: request.url,
      })),
      [
        {
          idempotencyKey: undefined,
          method: 'post',
          operation: 'image.generate',
          url: '/pg/models/quote',
        },
        {
          idempotencyKey: 'playground-generation-image',
          method: 'post',
          operation: 'image.generate',
          url: '/pg/chat/completions',
        },
        {
          idempotencyKey: 'playground-generation-video',
          method: 'post',
          operation: 'video.generate',
          url: '/pg/videos',
        },
        {
          idempotencyKey: undefined,
          method: 'get',
          operation: 'video.generate',
          url: '/pg/videos/task%2Fwith%20spaces',
        },
      ]
    )
  })
})

describe('media playground generation history', () => {
  test('uses the session history routes without dispatch headers', async () => {
    const requests: InternalAxiosRequestConfig[] = []
    const originalAdapter = api.defaults.adapter
    const record: PlaygroundGeneration = {
      id: '0123456789abcdef0123456789abcdef',
      operation: 'image',
      model: 'deepwl/gpt-image-2-all',
      group: 'default',
      prompt: 'A studio product photograph',
      parameters: { quality: 'low' },
      outputs: [],
      task_id: '',
      status: 'pending',
      error: '',
      contract_hash: 'contract-hash',
      contract_version: 1,
      pricing_version: 'pricing-version',
      quoted_quota: 8000,
      amount: 0.08,
      created_at: 1,
      updated_at: 1,
      completed_at: 0,
    }
    const adapter: AxiosAdapter = async (config) => {
      requests.push(config)
      const isList = config.method === 'get'
      const isDelete = config.method === 'delete'
      let responseData: unknown = { success: true, data: record }
      if (config.url?.endsWith('/assets')) {
        responseData = {
          success: true,
          data: {
            ordinal: 0,
            url: `/pg/generations/${record.id}/assets/0`,
            mime_type: 'image/png',
            size_bytes: 68,
          },
        }
      } else if (isDelete) {
        responseData = { success: true, data: null }
      } else if (isList) {
        responseData = {
          success: true,
          data: { items: [record], total: 1, page: 1, page_size: 20 },
        }
      }
      return {
        data: responseData,
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }
    api.defaults.adapter = adapter

    try {
      await getPlaygroundGenerations('image')
      await createPlaygroundGeneration({
        operation: 'image',
        model: record.model,
        group: record.group,
        prompt: record.prompt,
        parameters: record.parameters,
        contract_hash: record.contract_hash,
        contract_version: record.contract_version,
        pricing_version: record.pricing_version,
        quoted_quota: record.quoted_quota,
        amount: record.amount,
      })
      await updatePlaygroundGeneration(record.id, {
        status: 'succeeded',
        outputs: ['https://cdn.example.com/result.png'],
      })
      await uploadPlaygroundGenerationAsset(
        record.id,
        0,
        'data:image/png;base64,iVBORw0KGgo='
      )
      await deletePlaygroundGeneration(record.id)
    } finally {
      api.defaults.adapter = originalAdapter
    }

    assert.deepEqual(
      requests.map((request) => ({
        method: request.method,
        operation: request.headers.get('X-CarLab-Operation'),
        url: request.url,
      })),
      [
        { method: 'get', operation: undefined, url: '/pg/generations' },
        { method: 'post', operation: undefined, url: '/pg/generations' },
        {
          method: 'patch',
          operation: undefined,
          url: `/pg/generations/${record.id}`,
        },
        {
          method: 'post',
          operation: undefined,
          url: `/pg/generations/${record.id}/assets`,
        },
        {
          method: 'delete',
          operation: undefined,
          url: `/pg/generations/${record.id}`,
        },
      ]
    )
  })

  test('keeps only unique HTTP(S) result addresses', () => {
    assert.deepEqual(
      persistableMediaSources([
        'https://cdn.example.com/result.png',
        'https://cdn.example.com/result.png',
        'http://cdn.example.com/result.mp4',
        'data:image/png;base64,AAAA',
        'blob:https://api.carlab.top/result',
        '/relative.png',
      ]),
      [
        'https://cdn.example.com/result.png',
        'http://cdn.example.com/result.mp4',
      ]
    )
  })
})
