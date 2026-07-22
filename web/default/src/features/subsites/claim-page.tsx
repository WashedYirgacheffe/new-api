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
import { zodResolver } from '@hookform/resolvers/zod'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Building2, CheckCircle2, KeyRound } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { SectionPageLayout } from '@/components/layout'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Spinner } from '@/components/ui/spinner'
import { ROLE } from '@/lib/roles'

import { claimSubsite, getClaimableSubsites } from './api'
import { SubsiteRoleGuard } from './components/subsite-role-guard'

type ClaimFormValues = { password: string }

function createClaimFormSchema(translate: (key: string) => string) {
  return z.object({
    password: z.string().trim().min(1, translate('Claim password is required')),
  })
}

export function SubsiteClaimPage() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [selectedCode, setSelectedCode] = useState('')
  const claimFormSchema = useMemo(() => createClaimFormSchema(t), [t])
  const form = useForm<ClaimFormValues>({
    resolver: zodResolver(claimFormSchema),
    defaultValues: { password: '' },
  })
  const claimableQuery = useQuery({
    queryKey: ['subsites', 'claimable'],
    queryFn: getClaimableSubsites,
  })
  const sites = claimableQuery.data?.data?.items ?? []
  const activeSite =
    sites.find((site) => site.code === selectedCode) ?? sites[0]

  const claimMutation = useMutation({
    mutationFn: (values: ClaimFormValues) => {
      if (!activeSite) throw new Error(t('No subsite selected'))
      return claimSubsite(activeSite.code, values.password)
    },
    onSuccess: async (response) => {
      if (!response.success) return
      toast.success(t('Subsite claimed successfully'))
      form.reset()
      await queryClient.invalidateQueries({ queryKey: ['subsites'] })
    },
  })

  let claimStatusContent: ReactNode = null
  if (claimableQuery.isPending) {
    claimStatusContent = (
      <div className='flex min-h-32 items-center justify-center'>
        <Spinner />
      </div>
    )
  } else if (claimableQuery.isError || claimableQuery.data?.success === false) {
    claimStatusContent = (
      <Alert variant='destructive'>
        <AlertTitle>{t('Failed to load subsites')}</AlertTitle>
        <AlertDescription>
          {claimableQuery.data?.message || t('Please try again.')}
        </AlertDescription>
      </Alert>
    )
  } else if (sites.length === 0) {
    claimStatusContent = (
      <p className='text-muted-foreground py-8 text-center text-sm'>
        {t('No subsites available.')}
      </p>
    )
  }

  return (
    <SubsiteRoleGuard minimumRole={ROLE.SUBSITE_ADMIN}>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t('Claim Subsite')}</SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='mx-auto w-full max-w-3xl space-y-4'>
            <Alert>
              <KeyRound />
              <AlertTitle>{t('Subsite ownership')}</AlertTitle>
              <AlertDescription>
                {t(
                  'Use the management password issued for this subsite. A successful claim grants subsite model management access.'
                )}
              </AlertDescription>
            </Alert>

            <Card>
              <CardHeader>
                <CardTitle className='flex items-center gap-2'>
                  <Building2 className='size-4' />
                  {activeSite?.name ?? t('Subsite')}
                </CardTitle>
                <CardDescription>
                  {t('Claim a subsite with its management password.')}
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-5'>
                {claimStatusContent ?? (
                  <>
                    {sites.length > 1 && (
                      <div className='space-y-1.5'>
                        <label
                          className='text-sm font-medium'
                          htmlFor='claim-site'
                        >
                          {t('Subsite')}
                        </label>
                        <Select
                          value={activeSite?.code}
                          onValueChange={(value) =>
                            setSelectedCode(value ?? '')
                          }
                        >
                          <SelectTrigger id='claim-site' className='w-full'>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {sites.map((site) => (
                              <SelectItem key={site.code} value={site.code}>
                                {site.name} · {site.domain}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    )}

                    <dl className='grid gap-3 rounded-lg border p-4 sm:grid-cols-3'>
                      <div>
                        <dt className='text-muted-foreground text-xs'>
                          {t('Domain')}
                        </dt>
                        <dd className='mt-1 font-medium'>
                          {activeSite?.domain}
                        </dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground text-xs'>
                          {t('Version')}
                        </dt>
                        <dd className='mt-1 font-medium'>
                          {activeSite?.version}
                        </dd>
                      </div>
                      <div>
                        <dt className='text-muted-foreground text-xs'>
                          {t('Status')}
                        </dt>
                        <dd className='mt-1'>
                          <Badge
                            variant={
                              activeSite?.claimed ? 'secondary' : 'outline'
                            }
                          >
                            {activeSite?.claimed ? (
                              <CheckCircle2 data-icon='inline-start' />
                            ) : null}
                            {activeSite?.claimed
                              ? t('Claimed')
                              : t('Available to claim')}
                          </Badge>
                        </dd>
                      </div>
                    </dl>

                    <Form {...form}>
                      <form
                        className='space-y-4'
                        onSubmit={form.handleSubmit((values) =>
                          claimMutation.mutate(values)
                        )}
                      >
                        <FormField
                          control={form.control}
                          name='password'
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('Claim password')}</FormLabel>
                              <FormControl>
                                <Input
                                  type='password'
                                  autoComplete='current-password'
                                  placeholder={t('Enter claim password')}
                                  disabled={activeSite?.claimed}
                                  {...field}
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                        <Button
                          type='submit'
                          disabled={
                            activeSite?.claimed || claimMutation.isPending
                          }
                        >
                          {claimMutation.isPending ? <Spinner /> : <KeyRound />}
                          {claimMutation.isPending
                            ? t('Claiming...')
                            : t('Claim Subsite')}
                        </Button>
                      </form>
                    </Form>
                  </>
                )}
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
    </SubsiteRoleGuard>
  )
}
