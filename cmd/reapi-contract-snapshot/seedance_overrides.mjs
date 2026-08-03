const DURATION_VALUES = Array.from({ length: 12 }, (_, index) => index + 4)
const ASPECT_RATIOS = ['1:1', '4:3', '3:4', '16:9', '9:16', '21:9', 'adaptive']
const SEEDANCE_20_URL = 'https://reapi.ai/zh/models/seedance-2-0'
const SEEDANCE_20_MINI_URL = 'https://reapi.ai/zh/models/seedance-2-0-mini'

const faceMaterialSchema = {
  image: {
    min_items: 0,
    max_items: 11,
    request_fields: [
      {
        slot: 'image_urls',
        request_field: 'image_urls',
        transport: 'url',
        min_items: 0,
        max_items: 9,
        value_type: 'array',
      },
      {
        slot: 'first_frame',
        request_field: 'image_with_roles',
        transport: 'url',
        min_items: 0,
        max_items: 1,
        value_type: 'array',
        url_field: 'url',
        item_template: { role: 'first_frame' },
      },
      {
        slot: 'last_frame',
        request_field: 'image_with_roles',
        transport: 'url',
        min_items: 0,
        max_items: 1,
        value_type: 'array',
        url_field: 'url',
        item_template: { role: 'last_frame' },
      },
    ],
  },
  video: {
    max_items: 3,
    request_field: 'video_urls',
    transport: 'url',
  },
  audio: {
    max_items: 3,
    request_field: 'audio_urls',
    transport: 'url',
  },
}

function faceContract(resolutions) {
  const inputSchema = {
    type: 'object',
    properties: {
      prompt: { type: 'string' },
      duration: {
        type: 'integer',
        minimum: 4,
        maximum: 15,
        enum: DURATION_VALUES,
        default: 5,
      },
      size: {
        type: 'string',
        enum: ASPECT_RATIOS,
        default: '16:9',
      },
      resolution: {
        type: 'string',
        enum: resolutions,
        default: '720p',
      },
      generate_audio: { type: 'boolean', default: false },
      return_last_frame: { type: 'boolean', default: false },
      tools: { type: 'boolean', default: false },
      nsfw_checker: { type: 'boolean', default: true },
      seed: { type: 'integer' },
      fallback: {
        type: 'object',
        properties: {
          enabled: { type: 'boolean', default: true },
        },
        additionalProperties: false,
      },
    },
    additionalProperties: false,
  }
  return {
    input_schema: inputSchema,
    ui_schema: {
      order: Object.keys(inputSchema.properties),
      placements: {
        prompt: 'prompt',
        duration: 'footer',
        size: 'footer',
        resolution: 'footer',
        generate_audio: 'advanced',
        return_last_frame: 'advanced',
        tools: 'advanced',
        nsfw_checker: 'advanced',
        seed: 'advanced',
        fallback: 'hidden',
      },
      widgets: {
        prompt: 'textarea',
        duration: 'select',
        size: 'segmented',
        resolution: 'segmented',
        generate_audio: 'toggle',
        return_last_frame: 'toggle',
        tools: 'toggle',
        nsfw_checker: 'toggle',
        seed: 'stepper',
        fallback: 'hidden',
      },
    },
    material_schema: faceMaterialSchema,
    parameter_defaults: {
      duration: 5,
      size: '16:9',
      resolution: '720p',
      generate_audio: false,
      return_last_frame: false,
      tools: false,
      nsfw_checker: true,
    },
  }
}

const miniInputSchema = {
  type: 'object',
  properties: {
    prompt: { type: 'string' },
    generate_audio: { type: 'boolean', default: false },
    resolution: {
      type: 'string',
      enum: ['480p', '720p'],
      default: '720p',
    },
    aspect_ratio: {
      type: 'string',
      enum: ASPECT_RATIOS,
      default: '16:9',
    },
    duration: {
      type: 'integer',
      minimum: 4,
      maximum: 15,
      enum: DURATION_VALUES,
      default: 5,
    },
    web_search: { type: 'boolean', default: false },
    nsfw_checker: { type: 'boolean', default: true },
  },
  additionalProperties: false,
}

const miniContract = {
  input_schema: miniInputSchema,
  ui_schema: {
    order: Object.keys(miniInputSchema.properties),
    placements: {
      prompt: 'prompt',
      generate_audio: 'advanced',
      resolution: 'footer',
      aspect_ratio: 'footer',
      duration: 'footer',
      web_search: 'advanced',
      nsfw_checker: 'advanced',
    },
    widgets: {
      prompt: 'textarea',
      generate_audio: 'toggle',
      resolution: 'segmented',
      aspect_ratio: 'segmented',
      duration: 'select',
      web_search: 'toggle',
      nsfw_checker: 'toggle',
    },
  },
  material_schema: {
    image: {
      min_items: 0,
      max_items: 11,
      request_fields: [
        {
          slot: 'first_frame_url',
          request_field: 'first_frame_url',
          transport: 'url',
          min_items: 0,
          max_items: 1,
          value_type: 'string',
        },
        {
          slot: 'last_frame_url',
          request_field: 'last_frame_url',
          transport: 'url',
          min_items: 0,
          max_items: 1,
          value_type: 'string',
        },
        {
          slot: 'reference_image_urls',
          request_field: 'reference_image_urls',
          transport: 'url',
          min_items: 0,
          max_items: 9,
          value_type: 'array',
        },
      ],
    },
    video: {
      max_items: 3,
      request_field: 'reference_video_urls',
      transport: 'url',
    },
    audio: {
      max_items: 3,
      request_field: 'reference_audio_urls',
      transport: 'url',
    },
  },
  parameter_defaults: {
    generate_audio: false,
    resolution: '720p',
    aspect_ratio: '16:9',
    duration: 5,
    web_search: false,
    nsfw_checker: true,
  },
}

const overrides = new Map([
  ['doubao-seedance-2.0-face', {
    contract: faceContract(['480p', '720p', '1080p', '4k']),
    sourceUrl: SEEDANCE_20_URL,
  }],
  ['doubao-seedance-2.0-fast-face', {
    contract: faceContract(['480p', '720p']),
    sourceUrl: SEEDANCE_20_URL,
  }],
  ['seedance-2.0-mini', {
    contract: miniContract,
    sourceUrl: SEEDANCE_20_MINI_URL,
  }],
])

export function applySeedanceContractOverride(contract) {
  const override = overrides.get(contract.upstream_model_id)
  if (!override) return contract
  return {
    ...contract,
    ...structuredClone(override.contract),
    evidence: {
      ...contract.evidence,
      authoritative_override: {
        source_url: override.sourceUrl,
        documented_at: '2026-08-03',
        status: 'documented',
        precedence: 'specific_model_api_reference',
        url_inputs_only: true,
      },
    },
  }
}
