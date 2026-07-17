import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { PlaygroundCatalogBinding } from '../types'
import {
  extractImageOutputs,
  extractVideoTask,
  isPlaygroundBindingDispatchReady,
  missingRequiredParameter,
  playgroundDispatchPath,
  validateMaterials,
  type ParameterDescriptor,
} from './media-contract'

function binding(
  operation: PlaygroundCatalogBinding['operation'],
  adapter: string,
  dispatchPath = ''
): PlaygroundCatalogBinding {
  return {
    operation,
    profile_key: 'test.profile',
    profile_version: 1,
    contract_version: 1,
    contract_hash: 'contract-hash',
    endpoint_type: 'openai',
    execution_mode: 'sync',
    response_contract: 'test-response',
    dispatch_ready: true,
    effective_contract: {
      request_contract: { adapter },
      dispatch_path: dispatchPath,
    },
  }
}

describe('media playground dispatch contract', () => {
  test('routes supported image adapters to their matching playground endpoint', () => {
    assert.equal(
      playgroundDispatchPath(
        binding('image.generate', 'openai-chat'),
        'deepwl/chat-image'
      ),
      '/pg/chat/completions'
    )
    assert.equal(
      playgroundDispatchPath(
        binding('image.generate', 'openai-image'),
        'deepwl/gpt-image'
      ),
      '/pg/images/generations'
    )
  })

  test('requires video contracts to declare the supported dispatch path', () => {
    const incomplete = binding('video.generate', 'openai-video')
    assert.equal(
      isPlaygroundBindingDispatchReady(incomplete, 'deepwl/grok-video'),
      false
    )
    assert.throws(
      () => playgroundDispatchPath(incomplete, 'deepwl/grok-video'),
      /no supported dispatch path/
    )

    const supported = binding('video.generate', 'openai-video', '/v1/videos')
    assert.equal(
      playgroundDispatchPath(supported, 'deepwl/omni-fast'),
      '/pg/videos'
    )
  })
})

describe('media playground contract validation', () => {
  test('rejects a missing required rendered parameter', () => {
    const descriptors: ParameterDescriptor[] = [
      {
        name: 'quality',
        label: 'Quality',
        type: 'string',
        required: true,
        placement: 'advanced',
        widget: 'select',
        enumValues: ['low', 'high'],
      },
    ]
    assert.equal(missingRequiredParameter(descriptors, {}), 'Quality')
    assert.equal(
      missingRequiredParameter(descriptors, { quality: 'high' }),
      null
    )
  })

  test('extracts Markdown and data URI images from chat completions', () => {
    const outputs = extractImageOutputs({
      choices: [
        {
          message: {
            content:
              '![preview](https://cdn.example.com/output.png) data:image/png;base64,aGVsbG8=',
          },
        },
      ],
    })
    assert.deepEqual(
      outputs.map((output) => output.source),
      ['data:image/png;base64,aGVsbG8=', 'https://cdn.example.com/output.png']
    )
  })

  test('leaves MIME and size enforcement to the server probe', () => {
    const rules = {
      image: {
        kind: 'image' as const,
        minItems: 1,
        maxItems: 1,
        mimeTypes: ['image/png'],
        maxSizeMb: 15,
        requestField: 'images',
        transport: 'url',
      },
      video: {
        kind: 'video' as const,
        minItems: 0,
        maxItems: 0,
        mimeTypes: [],
        requestField: '',
        transport: '',
      },
      audio: {
        kind: 'audio' as const,
        minItems: 0,
        maxItems: 0,
        mimeTypes: [],
        requestField: '',
        transport: '',
      },
    }
    const issues = validateMaterials(rules, {
      image: [
        {
          id: 'material-1',
          source: 'https://cdn.example.com/reference.png',
          mime_type: '',
        },
      ],
      video: [],
      audio: [],
    })
    assert.deepEqual(issues, [])
  })

  test('parses the successful Playground video polling response', () => {
    assert.deepEqual(
      extractVideoTask({
        code: 'success',
        data: {
          task_id: 'task-video-1',
          status: 'SUCCESS',
          video_url: 'https://cdn.example.com/output.mp4',
        },
      }),
      {
        taskId: 'task-video-1',
        status: 'SUCCESS',
        progress: undefined,
        source: 'https://cdn.example.com/output.mp4',
        error: undefined,
      }
    )
  })
})
