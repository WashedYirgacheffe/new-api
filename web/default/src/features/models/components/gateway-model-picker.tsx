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
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  ComboboxInput,
  type ComboboxInputOption,
} from '@/components/ui/combobox-input'
import { useDebounce } from '@/hooks/use-debounce'

import { getModels, searchModels } from '../api'
import type { Model } from '../types'

const MODEL_SEARCH_DEBOUNCE_MS = 250
const MODEL_SEARCH_PAGE_SIZE = 25

type GatewayModelPickerProps = {
  value: string
  onValueChange: (value: string) => void
  onValueCommit: (value: string, model?: Model) => void
  className?: string
}

type ModelSearchRequest = {
  query: string
  revision: number
}

function modelChannels(model: Model): string[] {
  const channels = model.channel_providers?.filter(Boolean) || []
  if (channels.length > 0) return [...new Set(channels)]
  return [
    ...new Set(
      (model.bound_channels || [])
        .map((channel) => channel.channel_provider || channel.name)
        .filter(Boolean)
    ),
  ]
}

export function GatewayModelPicker(props: GatewayModelPickerProps) {
  const { t } = useTranslation()
  const [searchRequest, setSearchRequest] = useState<ModelSearchRequest>({
    query: '',
    revision: 0,
  })
  const [models, setModels] = useState<Model[]>([])
  const [loading, setLoading] = useState(true)
  const [searchError, setSearchError] = useState('')
  const searchQueryRef = useRef('')
  const requestRunIdRef = useRef(0)
  const debouncedSearchRequest = useDebounce(
    searchRequest,
    MODEL_SEARCH_DEBOUNCE_MS
  )

  useEffect(() => {
    const query = debouncedSearchRequest.query.trim()
    const runId = requestRunIdRef.current + 1
    requestRunIdRef.current = runId
    setLoading(true)
    setSearchError('')

    const load = async () => {
      try {
        const response = query
          ? await searchModels({
              keyword: query,
              p: 1,
              page_size: MODEL_SEARCH_PAGE_SIZE,
            })
          : await getModels({ p: 1, page_size: MODEL_SEARCH_PAGE_SIZE })
        if (
          runId !== requestRunIdRef.current ||
          query !== searchQueryRef.current.trim()
        ) {
          return
        }
        if (!response.success) {
          throw new Error(response.message || 'Model search failed.')
        }
        setModels(response.data?.items || [])
      } catch (error) {
        if (
          runId !== requestRunIdRef.current ||
          query !== searchQueryRef.current.trim()
        ) {
          return
        }
        setModels([])
        setSearchError(
          error instanceof Error ? error.message : 'Model search failed.'
        )
      } finally {
        if (
          runId === requestRunIdRef.current &&
          query === searchQueryRef.current.trim()
        ) {
          setLoading(false)
        }
      }
    }

    void load()
    return () => {
      if (runId === requestRunIdRef.current) {
        requestRunIdRef.current += 1
      }
    }
  }, [debouncedSearchRequest])

  const modelsByName = useMemo(
    () => new Map(models.map((model) => [model.model_name, model])),
    [models]
  )
  const options = useMemo<ComboboxInputOption[]>(
    () =>
      models.map((model) => {
        const channels = modelChannels(model)
        return {
          value: model.model_name,
          label: model.display_name?.trim() || model.model_name,
          description: model.model_name,
          metadata: `${t('Model Type')}: ${model.model_type || t('Unknown')} | ${t('Channel')}: ${channels.join(', ') || t('Unknown')}`,
        }
      }),
    [models, t]
  )

  const updateSearchQuery = useCallback((value: string) => {
    if (value === searchQueryRef.current) return
    searchQueryRef.current = value
    setSearchRequest((current) => ({
      query: value,
      revision: current.revision + 1,
    }))
    setLoading(true)
    setSearchError('')
  }, [])

  return (
    <ComboboxInput
      options={options}
      value={props.value}
      onValueChange={props.onValueChange}
      onValueCommit={(value) =>
        props.onValueCommit(value, modelsByName.get(value))
      }
      onSearchValueChange={updateSearchQuery}
      allowCustomValue
      shouldFilter={false}
      loading={loading}
      loadingText='Searching models...'
      emptyText={searchError || 'No gateway models found.'}
      placeholder={t('Search or enter an exact gateway model ID')}
      className={props.className}
    />
  )
}
