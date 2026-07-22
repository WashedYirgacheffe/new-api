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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Save } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import {
  getClaimableSubsites,
  getSubsiteModels,
  updateSubsiteModels,
} from './api'
import { ModelSelectionPanel } from './components/model-selection-panel'
import { SubsiteRoleGuard } from './components/subsite-role-guard'
import type { SubsiteCatalogModel, SubsiteModelsData } from './types'

const RUNTIME_ONLY_MODEL_IDS = new Set([
  'deepwl/omni-fast-v2v',
  'deepwl/grok-1.5-video-10s',
  'deepwl/grok-video-3-10s',
  'deepwl/grok-video-3-15s',
])

function sameModelIds(left: string[], right: string[]): boolean {
  if (left.length !== right.length) return false
  const rightSet = new Set(right)
  return left.every((modelId) => rightSet.has(modelId))
}

type ModelEditorProps = {
  code: string
  data: SubsiteModelsData
}

function ModelEditor(props: ModelEditorProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const models = useMemo<SubsiteCatalogModel[]>(
    () =>
      props.data.items.filter(
        (model) => !RUNTIME_ONLY_MODEL_IDS.has(model.model_id)
      ),
    [props.data.items]
  )
  const visibleModelIds = useMemo(
    () => new Set(models.map((model) => model.model_id)),
    [models]
  )
  const initialIds = useMemo(
    () =>
      props.data.enabled_model_ids.filter((modelId) =>
        visibleModelIds.has(modelId)
      ),
    [props.data.enabled_model_ids, visibleModelIds]
  )
  const [enabledModelIds, setEnabledModelIds] = useState(initialIds)
  const hasInvisibleStaleIds = props.data.enabled_model_ids.some(
    (modelId) => !visibleModelIds.has(modelId)
  )
  const hasChanges =
    hasInvisibleStaleIds || !sameModelIds(enabledModelIds, initialIds)
  const saveMutation = useMutation({
    mutationFn: () => updateSubsiteModels(props.code, enabledModelIds),
    onSuccess: async (response) => {
      if (!response.success) return
      toast.success(t('Enabled models saved'))
      await queryClient.invalidateQueries({
        queryKey: ['subsites', props.code, 'models'],
      })
    },
  })

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-3 border-b pb-3'>
        <div className='min-w-0'>
          <div className='flex flex-wrap items-center gap-2'>
            <h3 className='font-medium'>{props.data.site.name}</h3>
            <Badge variant='outline'>{props.data.site.version}</Badge>
          </div>
          <p className='text-muted-foreground mt-1 text-sm'>
            {props.data.site.domain} · {props.data.site.route_group}
          </p>
        </div>
        <Button
          onClick={() => saveMutation.mutate()}
          disabled={!hasChanges || saveMutation.isPending}
        >
          {saveMutation.isPending ? <Spinner /> : <Save />}
          {saveMutation.isPending ? t('Saving...') : t('Save')}
        </Button>
      </div>
      <ModelSelectionPanel
        models={models}
        enabledModelIds={enabledModelIds}
        onChange={setEnabledModelIds}
      />
    </div>
  )
}

export function SubsiteModelsPage() {
  const { t } = useTranslation()
  const role = useAuthStore((state) => state.auth.user?.role ?? 0)
  const [selectedCode, setSelectedCode] = useState('')
  const sitesQuery = useQuery({
    queryKey: ['subsites', 'claimable'],
    queryFn: getClaimableSubsites,
  })
  const manageableSites = useMemo(() => {
    const sites = sitesQuery.data?.data?.items ?? []
    if (role >= ROLE.ADMIN) return sites
    return sites.filter((site) => site.claimed)
  }, [role, sitesQuery.data?.data?.items])
  const activeSite =
    manageableSites.find((site) => site.code === selectedCode) ??
    manageableSites[0]
  const modelsQuery = useQuery({
    queryKey: ['subsites', activeSite?.code, 'models'],
    queryFn: () => {
      if (!activeSite) throw new Error(t('No subsite selected'))
      return getSubsiteModels(activeSite.code)
    },
    enabled: Boolean(activeSite),
  })

  let content: ReactNode
  const modelsPending = Boolean(activeSite) && modelsQuery.isPending
  if (sitesQuery.isPending || modelsPending) {
    content = (
      <div className='flex min-h-64 items-center justify-center'>
        <Spinner />
      </div>
    )
  } else if (sitesQuery.isError || sitesQuery.data?.success === false) {
    content = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Failed to load subsites')}</AlertTitle>
        <AlertDescription>
          {sitesQuery.data?.message || t('Please try again.')}
        </AlertDescription>
      </Alert>
    )
  } else if (!activeSite) {
    content = (
      <Alert>
        <AlertTitle>{t('No managed subsites')}</AlertTitle>
        <AlertDescription>
          {t('Claim a subsite before configuring its enabled models.')}
        </AlertDescription>
      </Alert>
    )
  } else if (modelsQuery.isError || modelsQuery.data?.success === false) {
    content = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Failed to load subsite models')}</AlertTitle>
        <AlertDescription>
          {modelsQuery.data?.message || t('Please try again.')}
        </AlertDescription>
      </Alert>
    )
  } else if (modelsQuery.data?.data) {
    content = (
      <ModelEditor
        key={activeSite.code}
        code={activeSite.code}
        data={modelsQuery.data.data}
      />
    )
  } else {
    content = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Failed to load subsite models')}</AlertTitle>
        <AlertDescription>
          {modelsQuery.data?.message || t('Please try again.')}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <SubsiteRoleGuard minimumRole={ROLE.SUBSITE_ADMIN}>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Enabled Models')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          {manageableSites.length > 0 && (
            <Select
              value={activeSite?.code}
              onValueChange={(value) => setSelectedCode(value ?? '')}
            >
              <SelectTrigger className='w-56 max-w-full'>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {manageableSites.map((site) => (
                  <SelectItem key={site.code} value={site.code}>
                    {site.name} · {site.domain}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>{content}</SectionPageLayout.Content>
      </SectionPageLayout>
    </SubsiteRoleGuard>
  )
}
