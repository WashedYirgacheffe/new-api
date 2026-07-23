import { createHash } from 'node:crypto'
import { readFile, writeFile } from 'node:fs/promises'
import { resolve } from 'node:path'

async function loadZod() {
  try {
    return (await import('../../web/node_modules/zod/index.js')).z
  } catch (error) {
    throw new Error(
      'Zod is required to extract the immutable RE manifests; run `cd web && bun install` first',
      { cause: error }
    )
  }
}

const z = await loadZod()

const DEFAULT_CATALOG = 'docs/catalog/reapi-model-catalog.json'
const DEFAULT_PRICING = 'docs/catalog/reapi-async-pricing.json'
const DEFAULT_OUTPUT = 'docs/catalog/reapi-async-contracts.json'
const OPENAPI_URL = 'https://reapi.ai/openapi.json'
const LLMS_URL = 'https://reapi.ai/llms.txt'
const MODELS_URL = 'https://reapi.ai/models'
const MANIFEST_CHUNK_URL =
  'https://reapi.ai/_next/static/chunks/b94c281bb89d522e.js'

const EXPECTED_OPENAPI_SHA256 =
  '0090c53338a3edcb252c99ffc51d46031f5929caf6938724ebce79c9eec2ce72'
const EXPECTED_LLMS_SHA256 =
  'd2966c605c14fad7f9d0f6575ce9de5c225fc590b9d6d5e375f4d3d24c8eaacd'
const EXPECTED_PRICING_SNAPSHOT_SHA256 =
  '2ecde5f1777b9380b7f4915b16341ba520c39b2cba902ed022d42df8911e1e33'
const EXPECTED_MANIFEST_CHUNK_SHA256 =
  'e12f6c3f98ce11593d3fc7eb03044563fa03f212d68a8a626def88311dd64b31'

const DEFERRED_BILLING_MODELS = new Set([
  're/audio-multistem',
  're/audio-music-extractor',
  're/audio-stem-separator',
  're/audio-voice-change',
  're/audio-voice-clean',
  're/enhance-video-1.0',
  're/topaz-video-upscaler',
])

const OPERATION_BY_TYPE = {
  image: 'image.generate',
  video: 'video.generate',
  audio: 'audio.generate',
  text: 'text.generate',
}

const RESPONSE_CONTRACT_BY_TYPE = {
  image: 're-image-task-v1',
  video: 're-video-task-v1',
  audio: 're-audio-task-v1',
  text: 're-text-task-v1',
}

function parseArgs(argv) {
  const options = {
    catalog: DEFAULT_CATALOG,
    pricing: DEFAULT_PRICING,
    output: DEFAULT_OUTPUT,
    generatedAt: '',
    check: false,
  }
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index]
    if (arg === '--check') {
      options.check = true
      continue
    }
    const value = argv[index + 1]
    if (!value) throw new Error(`${arg} requires a value`)
    if (arg === '--catalog') options.catalog = value
    else if (arg === '--pricing-catalog') options.pricing = value
    else if (arg === '--output') options.output = value
    else if (arg === '--generated-at') options.generatedAt = value
    else throw new Error(`unsupported argument ${arg}`)
    index += 1
  }
  return options
}

function sha256(value) {
  return createHash('sha256').update(value).digest('hex')
}

function canonicalJSON(value) {
  if (Array.isArray(value)) return value.map(canonicalJSON)
  if (!value || typeof value !== 'object') return value
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map((key) => [key, canonicalJSON(value[key])])
  )
}

function canonicalJSONString(value) {
  return JSON.stringify(canonicalJSON(value))
}

function assert(condition, message) {
  if (!condition) throw new Error(message)
}

function assertHash(actual, expected, source) {
  assert(
    actual === expected,
    `${source} SHA256 changed: expected ${expected}, got ${actual}`
  )
}

async function fetchBuffer(url) {
  let lastError = null
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      const response = await fetch(url, {
        headers: { 'User-Agent': 'CarLabAPI-reapi-contract-snapshot/1.0' },
      })
      if (!response.ok) throw new Error(`HTTP ${response.status}`)
      return Buffer.from(await response.arrayBuffer())
    } catch (error) {
      lastError = error
      if (attempt < 3) {
        await new Promise((resolveDelay) => setTimeout(resolveDelay, attempt * 250))
      }
    }
  }
  throw new Error(`fetch ${url}: ${lastError instanceof Error ? lastError.message : lastError}`)
}

async function mapWithConcurrency(values, concurrency, callback) {
  const results = new Array(values.length)
  let nextIndex = 0
  async function worker() {
    while (nextIndex < values.length) {
      const index = nextIndex
      nextIndex += 1
      results[index] = await callback(values[index], index)
    }
  }
  await Promise.all(Array.from({ length: concurrency }, () => worker()))
  return results
}

function extractBalancedObject(text, marker) {
  const markerIndex = text.indexOf(marker)
  if (markerIndex < 0) return null
  let start = markerIndex + marker.length
  while (/\s/.test(text[start])) start += 1
  if (text[start] !== '{') return null
  let depth = 0
  let inString = false
  let escaped = false
  for (let index = start; index < text.length; index += 1) {
    const character = text[index]
    if (inString) {
      if (escaped) escaped = false
      else if (character === '\\') escaped = true
      else if (character === '"') inString = false
      continue
    }
    if (character === '"') {
      inString = true
      continue
    }
    if (character === '{') depth += 1
    if (character === '}' && --depth === 0) {
      return text.slice(start, index + 1)
    }
  }
  return null
}

function parseModelPage(page) {
  const frames = []
  const pattern = /self\.__next_f\.push\((.*?)\)<\/script>/gs
  for (const match of page.matchAll(pattern)) {
    try {
      const frame = JSON.parse(match[1])
      if (typeof frame?.[1] === 'string') frames.push(frame[1])
    } catch {
      // Non-data frames do not contain the model configuration.
    }
  }
  const raw = extractBalancedObject(frames.join(''), '"cfg":')
  if (!raw) throw new Error('model page does not contain cfg')
  return JSON.parse(raw)
}

function llmsModelURLs(llms) {
  const urls = []
  for (const match of llms.matchAll(/\]\((\/models\/[^)]+)\)/g)) {
    urls.push(new URL(match[1], MODELS_URL).toString())
  }
  return [...new Set(urls)]
}

function modelIDFromCurl(curl) {
  if (typeof curl !== 'string') return ''
  return curl.match(/"model"\s*:\s*"([^"]+)"/)?.[1] || ''
}

function cfgChannels(cfg) {
  return Array.isArray(cfg?.channels) ? cfg.channels : []
}

function buildCfgModelMap(pages) {
  const byModel = new Map()
  for (const page of pages) {
    const baseModel = modelIDFromCurl(page.cfg?.apiReference?.minimalCurl)
    if (baseModel) {
      byModel.set(baseModel, { ...page, mapping: 'minimal_curl', channel: null })
    }
    for (const channel of cfgChannels(page.cfg)) {
      const model = String(channel?.submitModelId || '').trim()
      if (!model) continue
      byModel.set(model, { ...page, mapping: 'channel', channel })
    }
  }
  return byModel
}

function registerExports(modules, definitions, moduleID) {
  const target = modules.get(moduleID) || {}
  for (let index = 0; index < definitions.length; ) {
    const name = definitions[index]
    index += 1
    if (definitions[index] === 0) {
      index += 1
      target[name] = definitions[index]
      index += 1
      continue
    }
    const getter = definitions[index]
    index += 1
    Object.defineProperty(target, name, {
      enumerable: true,
      get: typeof getter === 'function' ? getter : () => getter,
    })
  }
  modules.set(moduleID, target)
}

function extractChunkManifests(source) {
  let frame = null
  const previousTurbopack = globalThis.TURBOPACK
  const previousManifests = globalThis.__REAPI_CONTRACT_MANIFESTS__
  globalThis.TURBOPACK = { push(value) { frame = value } }
  try {
    ;(0, eval)(source)
    assert(Array.isArray(frame), 'manifest chunk did not register a Turbopack frame')
    const modules = new Map([
      [207653, { z }],
      [469811, { ALL_SEED_SKUS: {}, PRODUCTS: {} }],
      [9127, {}],
      [921062, {}],
      [390908, { countWords: (text) => String(text).trim().split(/\s+/).filter(Boolean).length }],
      [955621, { E: { MISSING_PARAMETER: 'missing_parameter', PROMPT_TOO_LONG: 'prompt_too_long' } }],
      [725974, { gptImage2OfficialPriceUsd: {} }],
      [224993, {}],
      [37107, { calculateFromProduct: () => null }],
    ])
    const runtime = () => ({
      i(moduleID) {
        assert(modules.has(moduleID), `unsupported manifest import ${moduleID}`)
        return modules.get(moduleID)
      },
      s(definitions, moduleID) {
        registerExports(modules, definitions, moduleID)
      },
    })
    const sharedFactory = frame[frame.indexOf(568399) + 1]
    const auxiliaryFactory = frame[frame.indexOf(933740) + 1]
    const manifestFactory = frame[frame.indexOf(714540) + 1]
    assert(typeof sharedFactory === 'function', 'shared schema factory was not found')
    assert(typeof auxiliaryFactory === 'function', 'auxiliary schema factory was not found')
    assert(typeof manifestFactory === 'function', 'manifest factory was not found')
    sharedFactory(runtime())
    auxiliaryFactory(runtime())
    const marker = ';function t4('
    let factorySource = manifestFactory.toString()
    assert(factorySource.includes(marker), 'manifest export marker was not found')
    factorySource = factorySource.replace(
      marker,
      ';globalThis.__REAPI_CONTRACT_MANIFESTS__=t3;function t4('
    )
    ;(0, eval)(`(${factorySource})`)(runtime())
    const manifests = globalThis.__REAPI_CONTRACT_MANIFESTS__
    assert(manifests && typeof manifests === 'object', 'manifest map was not exported')
    return manifests
  } finally {
    if (previousTurbopack === undefined) delete globalThis.TURBOPACK
    else globalThis.TURBOPACK = previousTurbopack
    if (previousManifests === undefined) delete globalThis.__REAPI_CONTRACT_MANIFESTS__
    else globalThis.__REAPI_CONTRACT_MANIFESTS__ = previousManifests
  }
}

function zodDefinition(schema) {
  return schema?._zod?.def || schema?._def || null
}

function zodEnum(schema, seen = new Set()) {
  if (!schema || seen.has(schema)) return []
  seen.add(schema)
  const definition = zodDefinition(schema)
  if (!definition) return []
  if (definition.type === 'enum') return [...new Set(Object.values(definition.entries))]
  if (definition.type === 'literal') return [...definition.values]
  if (definition.type === 'union') {
    return [...new Set(definition.options.flatMap((option) => zodEnum(option, seen)))]
  }
  if (definition.type === 'pipe') {
    const output = zodEnum(definition.out, seen)
    return output.length > 0 ? output : zodEnum(definition.in, seen)
  }
  for (const key of ['innerType', 'schema', 'type']) {
    if (definition[key] && typeof definition[key] === 'object') {
      const values = zodEnum(definition[key], seen)
      if (values.length > 0) return values
    }
  }
  return []
}

function zodObjectShape(schema, seen = new Set()) {
  if (!schema || seen.has(schema)) return null
  seen.add(schema)
  const definition = zodDefinition(schema)
  if (!definition) return null
  if (definition.type === 'object') {
    return typeof definition.shape === 'function' ? definition.shape() : definition.shape
  }
  for (const key of ['innerType', 'schema', 'in', 'out']) {
    const shape = zodObjectShape(definition[key], seen)
    if (shape) return shape
  }
  return null
}

function zodObjectShapes(schema, seen = new Set()) {
  if (!schema || seen.has(schema)) return []
  seen.add(schema)
  const definition = zodDefinition(schema)
  if (!definition) return []
  if (definition.type === 'object') {
    const shape = typeof definition.shape === 'function' ? definition.shape() : definition.shape
    return [shape]
  }
  if (definition.type === 'union') {
    return definition.options.flatMap((option) => zodObjectShapes(option, seen))
  }
  for (const key of ['innerType', 'schema', 'in', 'out']) {
    const shapes = zodObjectShapes(definition[key], seen)
    if (shapes.length > 0) return shapes
  }
  return []
}

function zodArrayElement(schema, seen = new Set()) {
  if (!schema || seen.has(schema)) return null
  seen.add(schema)
  const definition = zodDefinition(schema)
  if (!definition) return null
  if (definition.type === 'array') return definition.element
  for (const key of ['innerType', 'schema', 'in', 'out']) {
    const element = zodArrayElement(definition[key], seen)
    if (element) return element
  }
  return null
}

function zodUnionOptions(schema, seen = new Set()) {
  if (!schema || seen.has(schema)) return []
  seen.add(schema)
  const definition = zodDefinition(schema)
  if (!definition) return []
  if (definition.type === 'union') return definition.options
  for (const key of ['innerType', 'schema', 'in', 'out']) {
    const options = zodUnionOptions(definition[key], seen)
    if (options.length > 0) return options
  }
  return []
}

function collapseAnyOf(schema) {
  if (!Array.isArray(schema.anyOf)) return schema
  const nonNull = schema.anyOf.filter((branch) => branch.type !== 'null')
  if (nonNull.length === 1) {
    const { anyOf, ...outer } = schema
    return { ...nonNull[0], ...outer }
  }
  const types = [...new Set(nonNull.flatMap((branch) => branch.type || []))]
  if (types.length === 1) schema.type = types[0]
  else if (types.length > 1) schema.type = types
  const values = nonNull.flatMap((branch) =>
    Array.isArray(branch.enum)
      ? branch.enum
      : Object.hasOwn(branch, 'const')
        ? [branch.const]
        : []
  )
  if (values.length === nonNull.length) schema.enum = [...new Set(values)]
  schema.anyOf = nonNull
  return schema
}

function annotateVariantType(schema, keyword) {
  const branches = schema[keyword]
  if (!Array.isArray(branches)) return schema
  const nonNull = branches.filter((branch) => branch.type !== 'null')
  const types = [...new Set(nonNull.flatMap((branch) => branch.type || []))]
  if (types.length === 1) schema.type = types[0]
  else if (types.length > 1) schema.type = types
  schema[keyword] = nonNull
  return schema
}

function normalizeSchemaNode(raw, zodSchema = null, field = '') {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return raw
  let schema = { ...raw }
  delete schema.$schema
  if (Array.isArray(schema.anyOf)) {
    const options = zodUnionOptions(zodSchema)
    schema.anyOf = schema.anyOf.map((branch, index) =>
      normalizeSchemaNode(branch, options[index] || null, field)
    )
    schema = collapseAnyOf(schema)
  }
  if (Array.isArray(schema.oneOf)) {
    const options = zodUnionOptions(zodSchema)
    schema.oneOf = schema.oneOf.map((branch, index) =>
      normalizeSchemaNode(branch, options[index] || null, field)
    )
    schema = annotateVariantType(schema, 'oneOf')
  }
  if (Object.hasOwn(schema, 'const')) {
    schema.enum = [schema.const]
    delete schema.const
  }
  const recoveredEnum = zodEnum(zodSchema).filter((value) => value !== null)
  if (recoveredEnum.length > 0) schema.enum = recoveredEnum
  if (schema.properties && typeof schema.properties === 'object') {
    const shape = zodObjectShape(zodSchema) || {}
    schema.properties = Object.fromEntries(
      Object.entries(schema.properties).map(([name, child]) => [
        name,
        normalizeSchemaNode(child, shape[name] || null, name),
      ])
    )
  }
  if (schema.items && typeof schema.items === 'object') {
    schema.items = normalizeSchemaNode(
      schema.items,
      zodArrayElement(zodSchema),
      `${field}[]`
    )
  }
  if (schema.additionalProperties && typeof schema.additionalProperties === 'object') {
    schema.additionalProperties = true
  }
  if (!schema.type && schema.properties) schema.type = 'object'
  if (!schema.type && schema.items) schema.type = 'array'
  if (!Array.isArray(schema.enum)) {
    const discovered = discoverFiniteIntegerEnum(zodSchema, field, schema)
    if (discovered.length > 0) schema.enum = discovered
  }
  return schema
}

function mergePropertySchemas(schemas) {
  if (schemas.length === 1) return schemas[0]
  if (schemas.every((schema) => canonicalJSONString(schema) === canonicalJSONString(schemas[0]))) {
    return schemas[0]
  }
  const result = { anyOf: schemas }
  const types = [...new Set(schemas.flatMap((schema) => schema.type || []))]
  if (types.length === 1) result.type = types[0]
  else if (types.length > 1) result.type = types
  if (schemas.every((schema) => Array.isArray(schema.enum))) {
    result.enum = [...new Set(schemas.flatMap((schema) => schema.enum))]
  }
  for (const bound of ['minimum', 'minLength', 'minItems']) {
    if (schemas.every((schema) => typeof schema[bound] === 'number')) {
      result[bound] = Math.min(...schemas.map((schema) => schema[bound]))
    }
  }
  for (const bound of ['maximum', 'maxLength', 'maxItems']) {
    if (schemas.every((schema) => typeof schema[bound] === 'number')) {
      result[bound] = Math.max(...schemas.map((schema) => schema[bound]))
    }
  }
  if (schemas.every((schema) => schema.default !== undefined)) {
    const defaults = [...new Set(schemas.map((schema) => canonicalJSONString(schema.default)))]
    if (defaults.length === 1) result.default = schemas[0].default
  }
  if (schemas.every((schema) => schema.format && schema.format === schemas[0].format)) {
    result.format = schemas[0].format
  }
  return result
}

function summarizeSchemaVariants(schema) {
  const keyword = Array.isArray(schema.oneOf)
    ? 'oneOf'
    : Array.isArray(schema.anyOf)
      ? 'anyOf'
      : ''
  if (!keyword) return []
  return schema[keyword]
    .filter((branch) => branch?.type === 'object')
    .map((branch) => {
      const action = branch.properties?.action?.enum
      return {
        ...(Array.isArray(action) && action.length === 1 ? { action: action[0] } : {}),
        required: branch.required || [],
        fields: Object.keys(branch.properties || {}),
      }
    })
}

function mergedObjectSchema(schema) {
  if (schema.properties) return schema
  const keyword = Array.isArray(schema.oneOf) ? 'oneOf' : 'anyOf'
  const branches = (schema[keyword] || []).filter((branch) => branch?.type === 'object')
  assert(
    branches.length > 0,
    `manifest JSON Schema has no object branch: ${JSON.stringify(schema).slice(0, 1000)}`
  )
  const propertyNames = [...new Set(branches.flatMap((branch) => Object.keys(branch.properties || {})))]
  const properties = Object.fromEntries(
    propertyNames.map((field) => [
      field,
      mergePropertySchemas(
        branches.flatMap((branch) =>
          branch.properties?.[field] ? [branch.properties[field]] : []
        )
      ),
    ])
  )
  const requiredSets = branches.map((branch) => new Set(branch.required || []))
  const required = propertyNames.filter((field) =>
    requiredSets.every((fields) => fields.has(field))
  )
  return {
    type: 'object',
    properties,
    required,
    additionalProperties: branches.every((branch) => branch.additionalProperties === false)
      ? false
      : true,
  }
}

function removeModelProperty(schema) {
  if (!schema || typeof schema !== 'object') return
  if (schema.properties) delete schema.properties.model
  if (Array.isArray(schema.required)) {
    schema.required = schema.required.filter((field) => field !== 'model')
  }
  for (const keyword of ['anyOf', 'oneOf']) {
    for (const branch of schema[keyword] || []) removeModelProperty(branch)
  }
}

function discoverFiniteIntegerEnum(zodSchema, field, schema) {
  if (!zodSchema || (schema.type !== 'integer' && schema.type !== 'number')) return []
  const isDuration = /^(duration|seconds)$/i.test(field)
  const isQuantity = /^(n|count|quantity|batch_size|.+_count|num_.+|number_of_.+)$/i.test(field)
  if (!isDuration && !isQuantity) return []
  const upper = isDuration ? 60 : 100
  const accepted = []
  for (let value = 0; value <= upper + 1; value += 1) {
    if (zodSchema.safeParse(value).success) accepted.push(value)
  }
  if (accepted.length === 0 || accepted.includes(upper + 1)) return []
  return accepted.filter((value) => value <= upper)
}

function enumValueKey(value) {
  return canonicalJSONString(value)
}

function sameEnumSet(left, right) {
  const leftKeys = new Set(left.map(enumValueKey))
  const rightKeys = new Set(right.map(enumValueKey))
  return leftKeys.size === rightKeys.size && [...leftKeys].every((value) => rightKeys.has(value))
}

function uniqueEnum(values) {
  const seen = new Set()
  return values.filter((value) => {
    const key = enumValueKey(value)
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

function zodAcceptsValue(schemas, value) {
  return schemas.some((schema) => schema.safeParse(value).success)
}

function schemaTypeAcceptsValue(type, value) {
  const types = Array.isArray(type) ? type : [type]
  return types.some((candidate) => {
    if (candidate === 'array') return Array.isArray(value)
    if (candidate === 'object') return value !== null && typeof value === 'object' && !Array.isArray(value)
    if (candidate === 'integer') return typeof value === 'number' && Number.isInteger(value)
    if (candidate === 'number') return typeof value === 'number' && Number.isFinite(value)
    if (candidate === 'null') return value === null
    return typeof value === candidate
  })
}

function finiteSelectorField(field) {
  const name = field.toLowerCase()
  if (
    [
      'n',
      'duration',
      'seconds',
      'aspect_ratio',
      'ratio',
      'size',
      'resolution',
      'quality',
      'image_count',
      'video_count',
      'output_count',
      'num_images',
      'num_videos',
      'num_outputs',
      'batch_size',
    ].includes(name)
  ) return true
  return (
    /_(duration|seconds|aspect_ratio|resolution|quality|count)$/.test(name) ||
    /^(num_|number_of_)/.test(name)
  )
}

function cfgFields(mapping) {
  const fields = mapping?.cfg?.playground?.params?.fields
  return Array.isArray(fields) ? fields : []
}

function coerceCfgValue(value, type) {
  if ((type === 'integer' || type === 'number') && typeof value === 'string') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : value
  }
  if (type === 'boolean' && typeof value === 'string') {
    if (value === 'true') return true
    if (value === 'false') return false
  }
  return value
}

function applyCfgFieldEvidence(inputSchema, mapping, shapes) {
  const properties = inputSchema.properties || {}
  const fields = new Map(cfgFields(mapping).map((field) => [field.name, field]))
  const channelOverrides = mapping?.channel?.enumOverrides || {}
  const mismatches = []
  for (const [name, schema] of Object.entries(properties)) {
    const field = fields.get(name)
    const override = channelOverrides[name]
    const rawEnum = Array.isArray(override)
      ? override
      : Array.isArray(field?.enumValues)
        ? field.enumValues
        : []
    const cfgEnum = uniqueEnum(rawEnum.map((value) => coerceCfgValue(value, schema.type)))
    if (cfgEnum.length > 0) {
      const manifestEnum = Array.isArray(schema.enum) ? [...schema.enum] : []
      const fieldSchemas = shapes.flatMap((shape) => shape[name] ? [shape[name]] : [])
      const effectiveEnum = fieldSchemas.length > 0
        ? cfgEnum.filter((value) => zodAcceptsValue(fieldSchemas, value))
        : manifestEnum.length > 0
          ? cfgEnum.filter((value) => new Set(manifestEnum.map(enumValueKey)).has(enumValueKey(value)))
          : cfgEnum
      if (effectiveEnum.length > 0) schema.enum = effectiveEnum
      if (manifestEnum.length > 0 && !sameEnumSet(manifestEnum, cfgEnum)) {
        mismatches.push({
          field: name,
          manifest_enum: manifestEnum,
          cfg_enum: cfgEnum,
          effective_enum: schema.enum,
        })
      }
    }
    if (schema.default === undefined && field?.default !== undefined) {
      schema.default = coerceCfgValue(field.default, schema.type)
    }
    if (schema.default !== undefined && !schemaTypeAcceptsValue(schema.type, schema.default)) {
      delete schema.default
    }
    if (
      Array.isArray(schema.enum) &&
      schema.default !== undefined &&
      !schema.enum.some((value) => enumValueKey(value) === enumValueKey(schema.default))
    ) {
      delete schema.default
    }
  }
  return mismatches
}

function pruneNonFiniteSelectors(schema, path = [], excluded = []) {
  if (!schema?.properties || typeof schema.properties !== 'object') return excluded
  for (const [field, fieldSchema] of Object.entries({ ...schema.properties })) {
    pruneNonFiniteSelectors(fieldSchema, [...path, field], excluded)
    if (!finiteSelectorField(field) || (Array.isArray(fieldSchema.enum) && fieldSchema.enum.length > 0)) {
      continue
    }
    delete schema.properties[field]
    if (Array.isArray(schema.required)) {
      schema.required = schema.required.filter((required) => required !== field)
    }
    excluded.push({
      field: [...path, field].join('.'),
      reason: 'finite_selector_without_official_enum',
    })
  }
  return excluded
}

function manifestInputSchema(manifest, mapping, modelID) {
  const raw = z.toJSONSchema(manifest.schema, {
    io: 'input',
    unrepresentable: 'any',
  })
  let input
  let schemaVariants = []
  try {
    const normalized = normalizeSchemaNode(raw, manifest.schema)
    schemaVariants = summarizeSchemaVariants(normalized)
    input = mergedObjectSchema(normalized)
  } catch (error) {
    throw new Error(`normalize manifest ${modelID}: ${error.message}`)
  }
  const shapes = zodObjectShapes(manifest.schema)
  removeModelProperty(input)
  for (const [field, schema] of Object.entries(input.properties)) {
    if (!Array.isArray(schema.enum)) {
      const fieldSchemas = shapes.flatMap((shape) => shape[field] ? [shape[field]] : [])
      const discovered = fieldSchemas.map((fieldSchema) =>
        discoverFiniteIntegerEnum(fieldSchema, field, schema)
      )
      if (discovered.length > 0 && discovered.every((values) => values.length > 0)) {
        schema.enum = [...new Set(discovered.flat())]
      }
    }
  }
  if (modelID === 'wan2.7-video' || modelID === 'wan2.7-video-official') {
    input.properties.duration.enum = [0, ...Array.from({ length: 14 }, (_, index) => index + 2)]
  }
  const enumMismatches = applyCfgFieldEvidence(input, mapping, shapes)
  const excludedInputFields = pruneNonFiniteSelectors(input)
  return { input, enumMismatches, schemaVariants, excludedInputFields }
}

function cfgMaterialKind(type) {
  if (typeof type !== 'string') return ''
  if (type.startsWith('image-')) return 'image'
  if (type.startsWith('video-')) return 'video'
  if (type.startsWith('audio-')) return 'audio'
  return ''
}

function inferredMaterialKind(field) {
  const name = field.toLowerCase()
  if (name === 'srt_url') return 'file'
  if (name === 'end_url') return 'image'
  if (name.includes('element_input_urls')) return 'image'
  if (/image|frame|mask/.test(name)) return 'image'
  if (/video|clip/.test(name)) return 'video'
  if (/audio|voice/.test(name)) return 'audio'
  return ''
}

function schemaBranches(schema) {
  return [
    schema,
    ...(schema.anyOf || []),
    ...(schema.oneOf || []),
    ...(schema.allOf || []),
  ].filter((branch) => branch && typeof branch === 'object')
}

function schemaURLValueType(schema) {
  for (const branch of schemaBranches(schema)) {
    if (branch.type === 'string' && branch.format === 'uri') return 'string'
    if (branch.type !== 'array') continue
    for (const item of schemaBranches(branch.items || {})) {
      if (item.format === 'uri' || item.properties?.url?.format === 'uri') return 'array'
    }
  }
  return ''
}

function schemaArrayItem(schema) {
  for (const branch of schemaBranches(schema)) {
    if (branch.type === 'array' && branch.items) return branch.items
  }
  return null
}

function schemaArrayBound(schema, bound, fallback) {
  const values = schemaBranches(schema)
    .filter((branch) => branch.type === 'array')
    .map((branch) => branch[bound])
  if (bound === 'minItems') {
    return values.length > 0 && values.every((value) => typeof value === 'number')
      ? Math.min(...values)
      : fallback
  }
  return values.length > 0 && values.every((value) => typeof value === 'number')
    ? Math.max(...values)
    : fallback
}

function collectURILocations(schema, path = [], arraySchema = null, locations = []) {
  if (!schema || typeof schema !== 'object') return locations
  if (schema.format === 'uri') locations.push({ path, array_schema: arraySchema })
  for (const [field, child] of Object.entries(schema.properties || {})) {
    collectURILocations(child, [...path, field], arraySchema, locations)
  }
  if (schema.items) collectURILocations(schema.items, [...path, '[]'], schema, locations)
  for (const keyword of ['allOf', 'anyOf', 'oneOf']) {
    for (const branch of schema[keyword] || []) {
      collectURILocations(branch, path, arraySchema, locations)
    }
  }
  return locations
}

function collectURIPaths(schema) {
  return collectURILocations(schema).map((location) => location.path)
}

function materialCandidate(inputSchema, mapping, name, schema, required) {
  const cfgField = new Map(cfgFields(mapping).map((field) => [field.name, field])).get(name)
  const kind = cfgMaterialKind(cfgField?.type) || inferredMaterialKind(name)
  const valueType = schemaURLValueType(schema)
  const itemSchema = schemaArrayItem(schema)
  const isMixed =
    name === 'media' &&
    itemSchema?.properties?.url?.format === 'uri' &&
    Array.isArray(itemSchema.properties?.type?.enum)
  if (!valueType && !isMixed) return null
  const roles = itemSchema?.properties?.role?.enum || []
  const minItems = valueType === 'array' || isMixed
    ? schemaArrayBound(schema, 'minItems', 0)
    : required
      ? 1
      : 0
  let maxItems = valueType === 'array' || isMixed
    ? schemaArrayBound(schema, 'maxItems', 20)
    : 1
  if (typeof cfgField?.max === 'number') maxItems = Math.min(maxItems, cfgField.max)
  return {
    field: name,
    kind: isMixed ? 'mixed' : kind || 'unsupported',
    cfg_type: cfgField?.type || null,
    required,
    min_items: Math.max(0, minItems),
    max_items: Math.min(20, maxItems),
    value_type: isMixed ? 'array' : valueType,
    roles,
    ...(itemSchema?.properties?.url?.format === 'uri' ? { item_schema: itemSchema } : {}),
    material_supported: isMixed || ['image', 'video', 'audio'].includes(kind),
    strip_field: name,
  }
}

function materialCandidates(inputSchema, mapping) {
  const required = new Set(inputSchema.required || [])
  const candidates = []
  for (const [name, schema] of Object.entries(inputSchema.properties || {})) {
    const candidate = materialCandidate(inputSchema, mapping, name, schema, required.has(name))
    if (candidate) candidates.push(candidate)
  }
  const coveredRoots = new Set(candidates.map((candidate) => candidate.field))
  for (const { path, array_schema: arraySchema } of collectURILocations(inputSchema)) {
    const root = path[0]
    if (!root || coveredRoots.has(root)) continue
    const field = path.filter((part) => part !== '[]').at(-1)
    candidates.push({
      field: path.join('.').replaceAll('.[]', '[]'),
      kind: inferredMaterialKind(path.join('_')) || 'unsupported',
      cfg_type: null,
      required: false,
      min_items: arraySchema?.minItems || 0,
      max_items: Math.min(20, arraySchema?.maxItems || 1),
      value_type: arraySchema ? 'array' : 'string',
      roles: [],
      material_supported: false,
      unsupported_reason: 'nested_material_path',
      strip_field: field,
    })
  }
  return candidates
}

function uniqueSlotName(slots, preferred, requestField) {
  const names = new Set(slots.map((slot) => slot.slot))
  if (!names.has(preferred)) return preferred
  const qualified = `${requestField}_${preferred}`
  assert(!names.has(qualified), `duplicate material slot ${qualified}`)
  return qualified
}

function ordinaryMaterialSlots(candidate, existing) {
  if (candidate.roles.length === 0) {
    return [{
      slot: uniqueSlotName(existing, candidate.field, candidate.field),
      request_field: candidate.field,
      transport: 'url',
      min_items: candidate.min_items,
      max_items: candidate.max_items,
      value_type: candidate.value_type,
    }]
  }
  return candidate.roles.map((role) => ({
    slot: uniqueSlotName(existing, role, candidate.field),
    request_field: candidate.field,
    transport: 'url',
    min_items: candidate.min_items >= candidate.roles.length ? 1 : 0,
    max_items: 1,
    value_type: 'array',
    url_field: 'url',
    item_template: { role },
  }))
}

function wanOfficialMediaSlots() {
  const slot = (name, kind, urlField = 'url', extra = {}) => ({
    kind,
    rule: {
      slot: name,
      request_field: 'media',
      transport: 'url',
      min_items: 0,
      max_items: 1,
      value_type: 'array',
      url_field: urlField,
      item_template: { type: name === 'reference_voice' ? 'driving_audio' : name },
      ...extra,
    },
  })
  return [
    slot('first_frame', 'image'),
    slot('last_frame', 'image'),
    slot('reference_image', 'image'),
    slot('first_clip', 'video'),
    slot('reference_video', 'video'),
    slot('video', 'video'),
    slot('driving_audio', 'audio'),
    slot('reference_voice', 'audio', 'reference_voice', { requires_slot: 'driving_audio' }),
  ]
}

function buildMaterialSchema(inputSchema, mapping) {
  const candidates = materialCandidates(inputSchema, mapping)
  const materialSchema = {}
  const slotsByKind = { image: [], video: [], audio: [] }
  for (const candidate of candidates) {
    if (candidate.kind === 'mixed' && candidate.field === 'media') {
      for (const { kind, rule } of wanOfficialMediaSlots()) slotsByKind[kind].push(rule)
      continue
    }
    if (!candidate.material_supported) continue
    slotsByKind[candidate.kind].push(
      ...ordinaryMaterialSlots(candidate, slotsByKind[candidate.kind])
    )
  }
  for (const kind of ['image', 'video', 'audio']) {
    const slots = slotsByKind[kind]
    if (slots.length === 0) continue
    const minItems = Math.min(20, slots.reduce((total, slot) => total + slot.min_items, 0))
    const maxItems = Math.min(20, slots.reduce((total, slot) => total + slot.max_items, 0))
    const legacy = slots.length === 1 &&
      !slots[0].url_field &&
      !slots[0].item_template &&
      !slots[0].requires_slot
    if (legacy) {
      materialSchema[kind] = {
        ...(minItems > 0 ? { min_items: minItems } : {}),
        max_items: maxItems,
        request_field: slots[0].request_field,
        transport: 'url',
      }
      continue
    }
    materialSchema[kind] = {
      min_items: minItems,
      max_items: maxItems,
      request_fields: slots,
    }
  }
  return { materialSchema, candidates }
}

function removeSchemaFieldRecursively(schema, field) {
  if (!schema || typeof schema !== 'object') return
  if (schema.properties && Object.hasOwn(schema.properties, field)) {
    delete schema.properties[field]
    if (Array.isArray(schema.required)) {
      schema.required = schema.required.filter((required) => required !== field)
    }
  }
  for (const child of Object.values(schema.properties || {})) {
    removeSchemaFieldRecursively(child, field)
  }
  if (schema.items) removeSchemaFieldRecursively(schema.items, field)
  for (const keyword of ['allOf', 'anyOf', 'oneOf']) {
    for (const branch of schema[keyword] || []) removeSchemaFieldRecursively(branch, field)
  }
}

function stripMaterialCandidateFields(inputSchema, candidates) {
  for (const candidate of candidates) {
    removeSchemaFieldRecursively(inputSchema, candidate.strip_field)
  }
}

function assertNoURLSchema(inputSchema, modelID) {
  const paths = collectURIPaths(inputSchema)
  assert(
    paths.length === 0,
    `${modelID} still exposes URL fields: ${paths.map((path) => path.join('.')).join(', ')}`
  )
}

function enumWidget(field, values) {
  if (finiteSelectorField(field)) {
    return values.length <= 8 ? 'segmented' : 'select'
  }
  return 'select'
}

function buildUISchema(inputSchema) {
  const order = Object.keys(inputSchema.properties || {})
  const placements = {}
  const widgets = {}
  for (const [field, schema] of Object.entries(inputSchema.properties || {})) {
    if (/^(prompt|input|text)$/i.test(field)) placements[field] = 'prompt'
    else if (/^(n|count|quantity|batch_size|.+_count|num_.+|number_of_.+)$/i.test(field)) placements[field] = 'batch'
    else if (/ratio|size|resolution|quality|duration|seconds/i.test(field)) placements[field] = 'footer'
    else if (schema.type === 'array' || schema.type === 'object') placements[field] = 'hidden'
    else placements[field] = 'advanced'

    if (Array.isArray(schema.enum) && schema.enum.length > 0) {
      widgets[field] = enumWidget(field, schema.enum)
    } else if (schema.type === 'boolean') widgets[field] = 'toggle'
    else if (schema.type === 'integer' || schema.type === 'number') widgets[field] = 'stepper'
    else if (schema.type === 'array' || schema.type === 'object') widgets[field] = 'hidden'
    else if (/prompt|input|text|lyrics|description/i.test(field)) widgets[field] = 'textarea'
    else widgets[field] = 'text'
  }
  return { order, placements, widgets }
}

function buildParameterDefaults(inputSchema) {
  const defaults = {}
  for (const [field, schema] of Object.entries(inputSchema.properties || {})) {
    if (schema.default === undefined) continue
    if (!['string', 'number', 'integer', 'boolean'].includes(schema.type)) continue
    defaults[field] = schema.default
  }
  return defaults
}

function materialCandidateEvidence(candidate) {
  const { strip_field, material_supported, ...evidence } = candidate
  return {
    ...evidence,
    ...(material_supported ? {} : { material_supported: false }),
  }
}

function collectCfgConstraints(mapping) {
  return cfgFields(mapping).flatMap((field) =>
    Array.isArray(field.constraints)
      ? field.constraints.map((constraint) => ({ field: field.name, ...constraint }))
      : []
  )
}

function collectZodConditions(schema, path = [], seen = new Set()) {
  if (!schema || seen.has(schema)) return []
  seen.add(schema)
  const definition = zodDefinition(schema)
  if (!definition) return []
  const conditions = []
  for (const check of definition.checks || []) {
    const checkDefinition = check?._zod?.def
    if (checkDefinition?.check !== 'custom') continue
    let message = ''
    if (typeof checkDefinition.error === 'function') {
      try { message = String(checkDefinition.error()) } catch { message = '' }
    }
    conditions.push({
      kind: message ? 'refine' : 'super_refine',
      path: [...path, ...(checkDefinition.path || [])],
      ...(message ? { message } : { source: 'immutable_manifest_chunk' }),
    })
  }
  const shape = zodObjectShape(schema)
  if (shape) {
    for (const [field, child] of Object.entries(shape)) {
      conditions.push(...collectZodConditions(child, [...path, field], seen))
    }
  }
  const element = zodArrayElement(schema)
  if (element) conditions.push(...collectZodConditions(element, [...path, '[]'], seen))
  for (const option of zodUnionOptions(schema)) {
    conditions.push(...collectZodConditions(option, path, seen))
  }
  return conditions
}

function cfgEvidence(mapping, inputSchema) {
  if (!mapping) return null
  const propertyNames = new Set(Object.keys(inputSchema.properties || {}))
  const fields = cfgFields(mapping).filter((field) => propertyNames.has(field.name))
  const modes = Array.isArray(mapping.cfg?.playground?.modes)
    ? mapping.cfg.playground.modes.map((mode) => ({
        id: mode.id,
        fields: (mode.fields || []).filter((field) => propertyNames.has(field)),
      }))
    : []
  const channel = mapping.channel
    ? {
        id: mapping.channel.id || null,
        submit_model_id: mapping.channel.submitModelId,
        fields: mapping.channel.fields || [],
        enum_overrides: mapping.channel.enumOverrides || {},
      }
    : null
  return {
    page_url: mapping.url,
    cfg_sha256: mapping.cfgSHA256,
    cfg_slug: mapping.cfg?.slug || null,
    mapping: mapping.mapping,
    endpoint_path: mapping.cfg?.apiReference?.endpointPath || null,
    channel,
    fields,
    modes,
    constraints: collectCfgConstraints(mapping),
  }
}

function openAPIModelIDs(openapi) {
  const models = new Set()
  function visit(value) {
    if (!value || typeof value !== 'object') return
    const values = value.properties?.model?.enum
    if (Array.isArray(values)) values.forEach((model) => models.add(model))
    Object.values(value).forEach(visit)
  }
  visit(openapi)
  return models
}

function sameSet(left, right) {
  return left.size === right.size && [...left].every((value) => right.has(value))
}

async function buildSnapshot(options, generatedAt) {
  const [catalogBuffer, pricingBuffer, openapiBuffer, llmsBuffer, modelsBuffer, chunkBuffer] =
    await Promise.all([
      readFile(resolve(options.catalog)),
      readFile(resolve(options.pricing)),
      fetchBuffer(OPENAPI_URL),
      fetchBuffer(LLMS_URL),
      fetchBuffer(MODELS_URL),
      fetchBuffer(MANIFEST_CHUNK_URL),
    ])
  const catalog = JSON.parse(catalogBuffer)
  const pricing = JSON.parse(pricingBuffer)
  const openapi = JSON.parse(openapiBuffer)
  const llms = llmsBuffer.toString('utf8')
  const modelsPage = modelsBuffer.toString('utf8')
  const chunkSource = chunkBuffer.toString('utf8')

  const openapiSHA256 = sha256(openapiBuffer)
  const llmsSHA256 = sha256(llmsBuffer)
  const chunkSHA256 = sha256(chunkBuffer)
  const pricingSnapshotRaw = modelsPage.match(
    /window\.__pricing_snapshot__=(\{.*?\});<\/script>/s
  )?.[1]
  assert(pricingSnapshotRaw, 'models index does not contain pricing snapshot')
  const pricingSnapshotSHA256 = sha256(pricingSnapshotRaw)
  assertHash(openapiSHA256, EXPECTED_OPENAPI_SHA256, OPENAPI_URL)
  assertHash(llmsSHA256, EXPECTED_LLMS_SHA256, LLMS_URL)
  assertHash(pricingSnapshotSHA256, EXPECTED_PRICING_SNAPSHOT_SHA256, MODELS_URL)
  assertHash(chunkSHA256, EXPECTED_MANIFEST_CHUNK_SHA256, MANIFEST_CHUNK_URL)

  const pageURLs = llmsModelURLs(llms)
  assert(pageURLs.length === 50, `expected 50 model pages, got ${pageURLs.length}`)
  const pages = await mapWithConcurrency(
    pageURLs,
    8,
    async (url) => {
      const pageBuffer = await fetchBuffer(url)
      const cfg = parseModelPage(pageBuffer.toString('utf8'))
      return {
        url,
        cfg,
        cfgSHA256: sha256(canonicalJSONString(cfg)),
      }
    }
  )
  const cfgByModel = buildCfgModelMap(pages)
  const manifests = extractChunkManifests(chunkSource)
  assert(Object.keys(manifests).length === 112, 'expected 112 manifest schemas')

  const asyncCatalog = catalog.models.filter((model) => model.protocol === 're-task')
  const asyncCatalogNames = new Set(asyncCatalog.map((model) => model.model_name))
  const asyncCatalogIDs = new Set(asyncCatalog.map((model) => model.upstream_model_id))
  const pricingNames = new Set(pricing.models.map((model) => model.model_name))
  const upstreamPublishableIDs = new Set(
    pricing.models
      .filter((model) => model.publishable)
      .map((model) => model.upstream_model_id)
  )
  assert(asyncCatalog.length === 96, `expected 96 async catalog models, got ${asyncCatalog.length}`)
  assert(sameSet(asyncCatalogNames, pricingNames), 'catalog and pricing async model sets differ')
  assert(
    sameSet(upstreamPublishableIDs, openAPIModelIDs(openapi)),
    'publishable pricing and OpenAPI model sets differ'
  )
  for (const model of asyncCatalog.filter((item) => upstreamPublishableIDs.has(item.upstream_model_id))) {
    assert(manifests[model.upstream_model_id], `missing manifest ${model.upstream_model_id}`)
  }

  const pricingByName = new Map(pricing.models.map((model) => [model.model_name, model]))
  const readyCatalog = asyncCatalog.filter((model) => {
    const price = pricingByName.get(model.model_name)
    return price?.publishable && !DEFERRED_BILLING_MODELS.has(model.model_name)
  })
  assert(readyCatalog.length === 88, `expected 88 publish-ready models, got ${readyCatalog.length}`)

  const models = readyCatalog
    .map((catalogModel) => {
      const manifest = manifests[catalogModel.upstream_model_id]
      const mapping = cfgByModel.get(catalogModel.upstream_model_id) || null
      const { input, enumMismatches, schemaVariants, excludedInputFields } = manifestInputSchema(
        manifest,
        mapping,
        catalogModel.upstream_model_id
      )
      const cfg = cfgEvidence(mapping, input)
      const { materialSchema, candidates } = buildMaterialSchema(input, mapping)
      stripMaterialCandidateFields(input, candidates)
      assertNoURLSchema(input, catalogModel.upstream_model_id)
      const operation = OPERATION_BY_TYPE[catalogModel.model_type]
      const responseContract = RESPONSE_CONTRACT_BY_TYPE[catalogModel.model_type]
      assert(operation && responseContract, `unsupported model type ${catalogModel.model_type}`)
      return {
        model_name: catalogModel.model_name,
        upstream_model_id: catalogModel.upstream_model_id,
        model_type: catalogModel.model_type,
        operation,
        endpoint_type: 're-task',
        execution_mode: 'async',
        response_contract: responseContract,
        schema_mode: 'replace',
        input_schema: input,
        ui_schema: buildUISchema(input),
        material_schema: materialSchema,
        request_contract: { adapter: 're-task', field_map: {}, coercions: {} },
        parameter_defaults: buildParameterDefaults(input),
        dispatch_path: '/v1/re/generations',
        poll_path: '/v1/re/tasks/{task_id}',
        evidence: {
          schema_source: 'immutable_manifest_chunk',
          manifest: {
            chunk_url: MANIFEST_CHUNK_URL,
            chunk_sha256: chunkSHA256,
            manifest_id: manifest.id,
            endpoint: manifest.endpoint,
          },
          cfg,
          material_candidates: candidates.map(materialCandidateEvidence),
          ...(schemaVariants.length > 0 ? { schema_variants: schemaVariants } : {}),
          ...(excludedInputFields.length > 0 ? { excluded_input_fields: excludedInputFields } : {}),
          enum_evidence_mismatches: enumMismatches,
          conditional_rules: collectZodConditions(manifest.schema),
        },
      }
    })
    .sort((left, right) => left.model_name.localeCompare(right.model_name))

  const exclusions = pricing.models
    .filter((model) => !models.some((contract) => contract.model_name === model.model_name))
    .map((model) => ({
      model_name: model.model_name,
      upstream_model_id: model.upstream_model_id,
      reason: DEFERRED_BILLING_MODELS.has(model.model_name)
        ? 'deferred_billing'
        : model.coming_soon
          ? 'coming_soon'
          : 'not_publishable',
      source_url: model.source_url,
    }))
    .sort((left, right) => left.model_name.localeCompare(right.model_name))
  assert(exclusions.length === 8, `expected 8 exclusions, got ${exclusions.length}`)

  const cfgMappedAsyncCount = asyncCatalog.filter((model) =>
    cfgByModel.has(model.upstream_model_id)
  ).length
  const cfgMappedReadyCount = readyCatalog.filter((model) =>
    cfgByModel.has(model.upstream_model_id)
  ).length
  assert(cfgMappedAsyncCount === 85, `expected 85 cfg-mapped async models, got ${cfgMappedAsyncCount}`)
  assert(cfgMappedReadyCount === 77, `expected 77 cfg-mapped ready models, got ${cfgMappedReadyCount}`)
  assert(new Set(models.map((model) => model.model_name)).size === 88, 'contract model IDs are not unique')
  const unsupportedFileMaterialCount = models.reduce(
    (count, model) => count + model.evidence.material_candidates.filter(
      (candidate) => candidate.kind === 'file' && candidate.material_supported === false
    ).length,
    0
  )
  assert(unsupportedFileMaterialCount === 1, 'expected one unsupported file material candidate')
  const sortedModelNameSHA256 = sha256(
    models.map((model) => model.model_name).sort().join('\n') + '\n'
  )

  const combinedCfgSHA256 = sha256(
    pages
      .map((page) => `${page.url}\t${page.cfgSHA256}`)
      .sort()
      .join('\n') + '\n'
  )
  return {
    schema_version: 1,
    generated_at: generatedAt,
    sources: {
      openapi: { url: OPENAPI_URL, sha256: openapiSHA256 },
      llms: { url: LLMS_URL, sha256: llmsSHA256 },
      models_index: {
        url: MODELS_URL,
        page_sha256: sha256(modelsBuffer),
        pricing_snapshot_sha256: pricingSnapshotSHA256,
      },
      model_pages: { count: pages.length, combined_cfg_sha256: combinedCfgSHA256 },
      manifest_chunk: { url: MANIFEST_CHUNK_URL, sha256: chunkSHA256 },
    },
    integrity: {
      model_count: models.length,
      exclusion_count: exclusions.length,
      sorted_model_name_sha256: sortedModelNameSHA256,
      unsupported_file_material_count: unsupportedFileMaterialCount,
      catalog_async_model_count: asyncCatalog.length,
      openapi_async_model_count: openAPIModelIDs(openapi).size,
      manifest_count: Object.keys(manifests).length,
      manifest_upstream_publishable_match_count: asyncCatalog.filter(
        (model) => manifests[model.upstream_model_id]
      ).length,
      cfg_mapped_async_model_count: cfgMappedAsyncCount,
      cfg_mapped_publish_ready_model_count: cfgMappedReadyCount,
      publish_ready_model_count: models.length,
      excluded_deferred_billing_count: exclusions.filter((item) => item.reason === 'deferred_billing').length,
      excluded_coming_soon_count: exclusions.filter((item) => item.reason === 'coming_soon').length,
    },
    exclusions,
    models,
  }
}

async function main() {
  const options = parseArgs(process.argv.slice(2))
  let existing = null
  if (options.check) existing = JSON.parse(await readFile(resolve(options.output), 'utf8'))
  const generatedAt = options.generatedAt || existing?.generated_at || new Date().toISOString()
  assert(!Number.isNaN(Date.parse(generatedAt)), 'generated-at must be RFC3339')
  const snapshot = await buildSnapshot(options, generatedAt)
  const output = `${JSON.stringify(snapshot, null, 2)}\n`
  if (options.check) {
    const current = `${JSON.stringify(existing, null, 2)}\n`
    assert(current === output, `${options.output} is stale; rerun the generator and review the diff`)
    console.log(`verified ${snapshot.models.length} RE async contracts in ${options.output}`)
    return
  }
  await writeFile(resolve(options.output), output)
  console.log(
    `wrote ${snapshot.models.length} RE async contracts and ${snapshot.exclusions.length} exclusions to ${options.output}`
  )
}

main().catch((error) => {
  console.error(error instanceof Error ? error.stack : error)
  process.exitCode = 1
})
