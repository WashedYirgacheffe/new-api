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
import { Search } from 'lucide-react'
import { useDeferredValue, useId, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

import type { SubsiteCatalogModel } from '../types'

const ALL_MODEL_TYPES = '__all__'

type ModelSelectionPanelProps = {
  models: SubsiteCatalogModel[]
  enabledModelIds: string[]
  onChange: (modelIds: string[]) => void
}

type ModelRowProps = {
  model: SubsiteCatalogModel
  selected: boolean
  onCheckedChange: (checked: boolean) => void
}

function isModelReady(model: SubsiteCatalogModel): boolean {
  return model.profile_ready && model.price_ready && model.routable
}

function ModelRow(props: ModelRowProps) {
  const { t } = useTranslation()
  const checkboxId = useId()
  const ready = isModelReady(props.model)

  return (
    <div
      className={cn(
        'flex min-w-0 items-start gap-3 border-b px-3 py-3 last:border-b-0',
        !ready && !props.selected && 'opacity-60'
      )}
    >
      <Checkbox
        id={checkboxId}
        className='mt-0.5'
        checked={props.selected}
        disabled={!ready && !props.selected}
        onCheckedChange={(checked) => props.onCheckedChange(checked === true)}
      />
      <Label
        htmlFor={checkboxId}
        className='min-w-0 flex-1 cursor-pointer flex-col items-start gap-1.5'
      >
        <span className='flex w-full min-w-0 items-center gap-2'>
          <span className='truncate font-medium'>
            {props.model.display_name || props.model.model_id}
          </span>
          <Badge variant={ready ? 'outline' : 'destructive'}>
            {ready ? t('Ready') : t('Not ready')}
          </Badge>
        </span>
        <span className='text-muted-foreground w-full truncate font-mono text-xs font-normal'>
          {props.model.model_id}
        </span>
        <span className='flex flex-wrap gap-1 font-normal'>
          <Badge variant='secondary'>{props.model.model_type}</Badge>
          <Badge
            variant={props.model.profile_ready ? 'outline' : 'destructive'}
          >
            {props.model.profile_ready
              ? t('Contract ready')
              : t('Contract not ready')}
          </Badge>
          <Badge variant={props.model.price_ready ? 'outline' : 'destructive'}>
            {props.model.price_ready ? t('Price ready') : t('Price not ready')}
          </Badge>
          <Badge variant={props.model.routable ? 'outline' : 'destructive'}>
            {props.model.routable ? t('Routable') : t('Not routable')}
          </Badge>
        </span>
      </Label>
    </div>
  )
}

export function ModelSelectionPanel(props: ModelSelectionPanelProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [modelType, setModelType] = useState(ALL_MODEL_TYPES)
  const deferredSearch = useDeferredValue(search.trim().toLowerCase())
  const enabledSet = useMemo(
    () => new Set(props.enabledModelIds),
    [props.enabledModelIds]
  )
  const modelTypes = useMemo(
    () =>
      [...new Set(props.models.map((model) => model.model_type))]
        .filter(Boolean)
        .sort(),
    [props.models]
  )
  const filteredModels = useMemo(
    () =>
      props.models.filter((model) => {
        if (modelType !== ALL_MODEL_TYPES && model.model_type !== modelType) {
          return false
        }
        if (!deferredSearch) return true
        const haystack = [
          model.model_id,
          model.display_name,
          model.model_provider,
          model.description,
        ]
          .filter(Boolean)
          .join(' ')
          .toLowerCase()
        return haystack.includes(deferredSearch)
      }),
    [deferredSearch, modelType, props.models]
  )
  const enabledModels = filteredModels.filter((model) =>
    enabledSet.has(model.model_id)
  )

  const toggleModel = (modelId: string, checked: boolean) => {
    const next = new Set(enabledSet)
    if (checked) next.add(modelId)
    else next.delete(modelId)
    props.onChange([...next].sort())
  }

  return (
    <div className='flex min-h-0 flex-col gap-4'>
      <div className='flex flex-col gap-2 sm:flex-row'>
        <div className='relative min-w-0 flex-1'>
          <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2' />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('Search models')}
            className='pl-8'
          />
        </div>
        <Select
          value={modelType}
          onValueChange={(value) => setModelType(value ?? ALL_MODEL_TYPES)}
        >
          <SelectTrigger className='w-full sm:w-48'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL_MODEL_TYPES}>{t('All Types')}</SelectItem>
            {modelTypes.map((type) => (
              <SelectItem key={type} value={type}>
                {type}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      <div className='grid min-h-0 gap-4 lg:grid-cols-2'>
        <section className='flex min-h-80 flex-col overflow-hidden rounded-lg border'>
          <header className='bg-muted/40 flex items-center justify-between border-b px-3 py-2.5'>
            <h3 className='text-sm font-medium'>{t('Available Models')}</h3>
            <span className='text-muted-foreground text-xs tabular-nums'>
              {filteredModels.length}
            </span>
          </header>
          <div className='max-h-[56vh] overflow-y-auto'>
            {filteredModels.length === 0 ? (
              <p className='text-muted-foreground px-4 py-12 text-center text-sm'>
                {t('No models match the current filters.')}
              </p>
            ) : (
              filteredModels.map((model) => (
                <ModelRow
                  key={model.model_id}
                  model={model}
                  selected={enabledSet.has(model.model_id)}
                  onCheckedChange={(checked) =>
                    toggleModel(model.model_id, checked)
                  }
                />
              ))
            )}
          </div>
        </section>

        <section className='flex min-h-80 flex-col overflow-hidden rounded-lg border'>
          <header className='bg-muted/40 flex items-center justify-between border-b px-3 py-2.5'>
            <h3 className='text-sm font-medium'>{t('Enabled Models')}</h3>
            <span className='text-muted-foreground text-xs tabular-nums'>
              {props.enabledModelIds.length}
            </span>
          </header>
          <div className='max-h-[56vh] overflow-y-auto'>
            {enabledModels.length === 0 ? (
              <p className='text-muted-foreground px-4 py-12 text-center text-sm'>
                {props.enabledModelIds.length > 0
                  ? t('No enabled models match the current filters.')
                  : t('No models enabled for this subsite.')}
              </p>
            ) : (
              enabledModels.map((model) => (
                <ModelRow
                  key={model.model_id}
                  model={model}
                  selected
                  onCheckedChange={(checked) =>
                    toggleModel(model.model_id, checked)
                  }
                />
              ))
            )}
          </div>
        </section>
      </div>
    </div>
  )
}
