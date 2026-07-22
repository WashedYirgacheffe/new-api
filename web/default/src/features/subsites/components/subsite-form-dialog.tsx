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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { z } from 'zod'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'

import { createSubsite, updateSubsite } from '../api'
import type { Subsite, SubsiteFormPayload } from '../types'

type SubsiteFormValues = {
  code: string
  name: string
  domain: string
  version: string
  route_group: string
  claim_password: string
  enabled: boolean
}

function createSubsiteFormSchema(translate: (key: string) => string) {
  return z.object({
    code: z.string().trim().min(1, translate('Code is required')),
    name: z.string().trim().min(1, translate('Name is required')),
    domain: z.string().trim().min(1, translate('Domain is required')),
    version: z.string().trim().min(1, translate('Version is required')),
    route_group: z.string().trim().min(1, translate('Route group is required')),
    claim_password: z.string(),
    enabled: z.boolean(),
  })
}

const DEFAULT_VALUES: SubsiteFormValues = {
  code: '',
  name: '',
  domain: '',
  version: '',
  route_group: '',
  claim_password: '',
  enabled: true,
}

type SubsiteFormDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  subsite?: Subsite
}

export function SubsiteFormDialog(props: SubsiteFormDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const isEditing = Boolean(props.subsite)
  const subsiteFormSchema = useMemo(() => createSubsiteFormSchema(t), [t])
  const form = useForm<SubsiteFormValues>({
    resolver: zodResolver(subsiteFormSchema),
    defaultValues: DEFAULT_VALUES,
  })

  useEffect(() => {
    if (!props.open) return
    if (!props.subsite) {
      form.reset(DEFAULT_VALUES)
      return
    }
    form.reset({
      code: props.subsite.code,
      name: props.subsite.name,
      domain: props.subsite.domain,
      version: props.subsite.version,
      route_group: props.subsite.route_group,
      claim_password: '',
      enabled: props.subsite.enabled,
    })
  }, [form, props.open, props.subsite])

  const saveMutation = useMutation({
    mutationFn: (values: SubsiteFormValues) => {
      const payload: SubsiteFormPayload = {
        id: props.subsite?.id,
        code: values.code,
        name: values.name,
        domain: values.domain,
        version: values.version,
        route_group: values.route_group,
        enabled: values.enabled,
      }
      const password = values.claim_password.trim()
      if (password) payload.claim_password = password
      if (props.subsite) return updateSubsite(props.subsite.code, payload)
      return createSubsite(payload)
    },
    onSuccess: async (response) => {
      if (!response.success) return
      toast.success(
        isEditing
          ? t('Subsite updated successfully')
          : t('Subsite created successfully')
      )
      props.onOpenChange(false)
      await queryClient.invalidateQueries({ queryKey: ['subsites'] })
    },
  })

  const onSubmit = (values: SubsiteFormValues) => {
    if (!isEditing && !values.claim_password.trim()) {
      form.setError('claim_password', {
        message: t('Claim password is required'),
      })
      return
    }
    saveMutation.mutate(values)
  }

  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent className='max-h-[85vh] overflow-y-auto sm:max-w-2xl'>
        <DialogHeader>
          <DialogTitle>
            {isEditing ? t('Edit Subsite') : t('Create Subsite')}
          </DialogTitle>
          <DialogDescription>
            {t(
              'Configure the subsite identity, routing group, and claim access.'
            )}
          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form
            id='subsite-form'
            className='space-y-4'
            onSubmit={form.handleSubmit(onSubmit)}
          >
            <div className='grid gap-4 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='code'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Code')}</FormLabel>
                    <FormControl>
                      <Input placeholder='superseed' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name')}</FormLabel>
                    <FormControl>
                      <Input placeholder='Superseed' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='domain'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Domain')}</FormLabel>
                    <FormControl>
                      <Input placeholder='cjzz.top' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='version'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Version')}</FormLabel>
                    <FormControl>
                      <Input placeholder='v0.9.0' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='route_group'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Route group')}</FormLabel>
                    <FormControl>
                      <Input placeholder='gold' {...field} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='claim_password'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Claim password')}</FormLabel>
                    <FormControl>
                      <Input
                        type='password'
                        autoComplete='new-password'
                        placeholder={
                          isEditing
                            ? t('Leave blank to keep current password')
                            : t('Enter claim password')
                        }
                        {...field}
                      />
                    </FormControl>
                    {isEditing && (
                      <FormDescription>
                        {t('The current password is never displayed.')}
                      </FormDescription>
                    )}
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
            <FormField
              control={form.control}
              name='enabled'
              render={({ field }) => (
                <FormItem className='flex items-center justify-between gap-4 rounded-lg border p-3'>
                  <div className='space-y-1'>
                    <FormLabel>{t('Enabled')}</FormLabel>
                    <FormDescription>
                      {t('Allow this subsite to receive its model catalog.')}
                    </FormDescription>
                  </div>
                  <FormControl>
                    <Switch
                      checked={field.value}
                      onCheckedChange={field.onChange}
                    />
                  </FormControl>
                </FormItem>
              )}
            />
          </form>
        </Form>
        <DialogFooter>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
            disabled={saveMutation.isPending}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='subsite-form'
            disabled={saveMutation.isPending}
          >
            {saveMutation.isPending && <Spinner />}
            {saveMutation.isPending ? t('Saving...') : t('Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
