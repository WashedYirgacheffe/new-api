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

import type {
  ModelContractObject,
  ModelOperationBinding,
  ModelOperationEffectiveContract,
} from '../types'
import { deriveModelContractCapabilities } from './model-contract-capabilities'

function binding(
  operation: string,
  inputSchema: ModelContractObject,
  materialSchema: ModelContractObject
): Pick<ModelOperationBinding, 'operation' | 'effective_contract'> {
  return {
    operation,
    effective_contract: {
      input_schema: inputSchema,
      material_schema: materialSchema,
    } as ModelOperationEffectiveContract,
  }
}

describe('model contract capability derivation', () => {
  test('derives required prompt and supported material quantities', () => {
    const capabilities = deriveModelContractCapabilities(
      { model_type: 'video' },
      binding(
        'video.generate',
        {
          properties: { prompt: { type: 'string' } },
          required: ['prompt'],
        },
        {
          image: { max_items: 5 },
          video: { min_items: 1, max_items: 1 },
          audio: { max_items: 0 },
        }
      )
    )

    assert.deepEqual(capabilities, {
      modelType: 'video',
      prompt: { accepted: true, required: true },
      textInput: { fields: [], required: false },
      materials: [
        { kind: 'image', minItems: 0, maxItems: 5 },
        { kind: 'video', minItems: 1, maxItems: 1 },
      ],
      outputKind: 'video',
      downstreamRule: { kind: 'material', materialKind: 'video' },
    })
  })

  test('maps text output to downstream prompt input', () => {
    const capabilities = deriveModelContractCapabilities(
      { model_type: 'text' },
      binding('text.chat', { properties: { prompt: { type: 'string' } } }, {})
    )

    assert.deepEqual(capabilities.prompt, {
      accepted: true,
      required: false,
    })
    assert.equal(capabilities.outputKind, 'text')
    assert.deepEqual(capabilities.downstreamRule, { kind: 'prompt' })
  })

  test('maps every published operation to its output kind', () => {
    const operations = [
      ['text.chat', 'text'],
      ['image.generate', 'image'],
      ['video.generate', 'video'],
      ['audio.generate', 'audio'],
      ['embedding.create', 'embedding'],
      ['rerank.create', 'ranking'],
    ] as const

    for (const [operation, outputKind] of operations) {
      const capabilities = deriveModelContractCapabilities(
        undefined,
        binding(operation, {}, {})
      )
      assert.equal(capabilities.outputKind, outputKind)
    }
  })

  test('treats an omitted material maximum as unbounded', () => {
    const capabilities = deriveModelContractCapabilities(
      undefined,
      binding('image.generate', {}, { image: { min_items: 1 } })
    )

    assert.deepEqual(capabilities.materials, [
      { kind: 'image', minItems: 1, maxItems: null },
    ])
  })

  test('groups published input, query, and documents fields as text input', () => {
    const capabilities = deriveModelContractCapabilities(
      { model_type: 'rerank' },
      binding(
        'rerank.create',
        {
          properties: {
            query: { type: 'string' },
            documents: { type: 'array' },
          },
          required: ['query', 'documents'],
        },
        {}
      )
    )

    assert.deepEqual(capabilities.textInput, {
      fields: ['query', 'documents'],
      required: true,
    })
  })

  test('does not invent capabilities for missing metadata or unknown operations', () => {
    const capabilities = deriveModelContractCapabilities(
      undefined,
      binding(
        'custom.operation',
        { required: ['prompt'] },
        { image: { min_items: 2, max_items: 0 } }
      )
    )

    assert.equal(capabilities.modelType, null)
    assert.deepEqual(capabilities.prompt, {
      accepted: false,
      required: false,
    })
    assert.deepEqual(capabilities.textInput, {
      fields: [],
      required: false,
    })
    assert.deepEqual(capabilities.materials, [])
    assert.equal(capabilities.outputKind, 'unknown')
    assert.deepEqual(capabilities.downstreamRule, { kind: 'none' })
  })
})
