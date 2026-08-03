import assert from 'node:assert/strict'
import test from 'node:test'

import { applySeedanceContractOverride } from './seedance_overrides.mjs'

function contract(model) {
  return applySeedanceContractOverride({
    upstream_model_id: model,
    evidence: { schema_source: 'fixture' },
  })
}

test('Face contracts expose only documented parameters', () => {
  const face = contract('doubao-seedance-2.0-face')
  const fastFace = contract('doubao-seedance-2.0-fast-face')

  assert.deepEqual(face.input_schema.properties.resolution.enum, ['480p', '720p', '1080p', '4k'])
  assert.deepEqual(fastFace.input_schema.properties.resolution.enum, ['480p', '720p'])
  for (const item of [face, fastFace]) {
    assert.equal(item.input_schema.required, undefined)
    assert.equal(item.input_schema.properties.nsfw_checker.type, 'boolean')
    assert.equal(item.input_schema.properties.tools.type, 'boolean')
    assert.equal(item.input_schema.properties.seed.type, 'integer')
    assert.equal(item.ui_schema.widgets.tools, 'toggle')
    assert.equal(item.ui_schema.widgets.nsfw_checker, 'toggle')
    assert.equal(item.evidence.authoritative_override.status, 'documented')
  }
})

test('Mini exposes nsfw_checker and omits return_last_frame', () => {
  const mini = contract('seedance-2.0-mini')

  assert.equal(mini.input_schema.required, undefined)
  assert.equal(mini.input_schema.properties.nsfw_checker.type, 'boolean')
  assert.equal(mini.ui_schema.widgets.nsfw_checker, 'toggle')
  assert.equal(mini.input_schema.properties.return_last_frame, undefined)
  assert.deepEqual(mini.input_schema.properties.resolution.enum, ['480p', '720p'])
})
