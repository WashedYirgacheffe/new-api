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
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ROLE } from '@/lib/roles'

import { deleteSubsite, getSubsites } from './api'
import { SubsiteFormDialog } from './components/subsite-form-dialog'
import { SubsiteRoleGuard } from './components/subsite-role-guard'
import type { Subsite } from './types'

export function SubsiteAdminPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [formOpen, setFormOpen] = useState(false)
  const [editingSubsite, setEditingSubsite] = useState<Subsite>()
  const [deletingSubsite, setDeletingSubsite] = useState<Subsite>()
  const subsitesQuery = useQuery({
    queryKey: ['subsites', 'admin'],
    queryFn: getSubsites,
  })
  const subsites = subsitesQuery.data?.data?.items ?? []
  const deleteMutation = useMutation({
    mutationFn: deleteSubsite,
    onSuccess: async (response) => {
      if (!response.success) return
      toast.success(t('Subsite deleted successfully'))
      setDeletingSubsite(undefined)
      await queryClient.invalidateQueries({ queryKey: ['subsites'] })
    },
  })

  const openCreate = () => {
    setEditingSubsite(undefined)
    setFormOpen(true)
  }

  const openEdit = (subsite: Subsite) => {
    setEditingSubsite(subsite)
    setFormOpen(true)
  }

  let statusContent: ReactNode = null
  if (subsitesQuery.isPending) {
    statusContent = (
      <div className='flex min-h-64 items-center justify-center'>
        <Spinner />
      </div>
    )
  } else if (subsitesQuery.isError || subsitesQuery.data?.success === false) {
    statusContent = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Failed to load subsites')}</AlertTitle>
        <AlertDescription>
          {subsitesQuery.data?.message || t('Please try again.')}
        </AlertDescription>
      </Alert>
    )
  } else if (subsites.length === 0) {
    statusContent = (
      <div className='flex min-h-64 flex-col items-center justify-center gap-3 rounded-lg border border-dashed p-6 text-center'>
        <p className='text-muted-foreground text-sm'>
          {t('No subsites configured.')}
        </p>
        <Button variant='outline' onClick={openCreate}>
          <Plus />
          {t('Create Subsite')}
        </Button>
      </div>
    )
  }

  return (
    <SubsiteRoleGuard minimumRole={ROLE.ADMIN}>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Subsites')}</SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button onClick={openCreate}>
            <Plus />
            {t('Create Subsite')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          {statusContent ?? (
            <div className='overflow-hidden rounded-lg border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('Name')}</TableHead>
                    <TableHead>{t('Code')}</TableHead>
                    <TableHead>{t('Domain')}</TableHead>
                    <TableHead>{t('Version')}</TableHead>
                    <TableHead>{t('Route group')}</TableHead>
                    <TableHead>{t('Status')}</TableHead>
                    <TableHead className='w-24 text-right'>
                      {t('Actions')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {subsites.map((subsite) => (
                    <TableRow key={subsite.id}>
                      <TableCell className='font-medium'>
                        {subsite.name}
                      </TableCell>
                      <TableCell className='font-mono'>
                        {subsite.code}
                      </TableCell>
                      <TableCell>{subsite.domain}</TableCell>
                      <TableCell>{subsite.version}</TableCell>
                      <TableCell>{subsite.route_group}</TableCell>
                      <TableCell>
                        <Badge
                          variant={subsite.enabled ? 'outline' : 'secondary'}
                        >
                          {subsite.enabled ? t('Enabled') : t('Disabled')}
                        </Badge>
                      </TableCell>
                      <TableCell>
                        <div className='flex justify-end gap-1'>
                          <Button
                            size='icon-sm'
                            variant='ghost'
                            aria-label={t('Edit Subsite')}
                            onClick={() => openEdit(subsite)}
                          >
                            <Pencil />
                          </Button>
                          <Button
                            size='icon-sm'
                            variant='ghost'
                            aria-label={t('Delete Subsite')}
                            onClick={() => setDeletingSubsite(subsite)}
                          >
                            <Trash2 />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <SubsiteFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        subsite={editingSubsite}
      />
      <ConfirmDialog
        open={Boolean(deletingSubsite)}
        onOpenChange={(open) => !open && setDeletingSubsite(undefined)}
        title={t('Delete Subsite')}
        desc={t(
          'Delete subsite {{name}}? Its claim and enabled model assignments will also be removed.',
          { name: deletingSubsite?.name ?? '' }
        )}
        confirmText={deleteMutation.isPending ? t('Deleting...') : t('Delete')}
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deletingSubsite) deleteMutation.mutate(deletingSubsite.code)
        }}
      />
    </SubsiteRoleGuard>
  )
}
