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
  getPlaygroundVideo,
  quotePlaygroundModel,
  runPlaygroundMedia,
} from './api'

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
        { model: 'deepwl/gpt-image-2' }
      )
      await runPlaygroundMedia('/pg/videos', 'default', 'video.generate', {
        model: 'deepwl/omni-fast-v2v',
      })
      await getPlaygroundVideo('task/with spaces', 'default', 'video.generate')
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
        {
          method: 'post',
          operation: 'image.generate',
          url: '/pg/models/quote',
        },
        {
          method: 'post',
          operation: 'image.generate',
          url: '/pg/chat/completions',
        },
        {
          method: 'post',
          operation: 'video.generate',
          url: '/pg/videos',
        },
        {
          method: 'get',
          operation: 'video.generate',
          url: '/pg/videos/task%2Fwith%20spaces',
        },
      ]
    )
  })
})
