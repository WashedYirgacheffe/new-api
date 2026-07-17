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
import {
  Clock3,
  ImageIcon,
  Loader2,
  Maximize2,
  RefreshCw,
  Trash2,
  TriangleAlert,
  Video,
} from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { PlaygroundGeneration } from '../types'
import type { MediaPreview } from './media-preview-dialog'

function generationStatusLabel(
  status: PlaygroundGeneration['status'],
  t: (key: string) => string
): string {
  if (status === 'succeeded') return t('Succeeded')
  if (status === 'failed') return t('Failed')
  return t('Pending')
}

function generationStatusVariant(
  status: PlaygroundGeneration['status']
): 'default' | 'secondary' | 'destructive' {
  if (status === 'succeeded') return 'default'
  if (status === 'failed') return 'destructive'
  return 'secondary'
}

function generationTime(timestamp: number): string {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp * 1000))
}

function generationAmount(amount: number): string {
  if (!Number.isFinite(amount) || amount < 0) return '-'
  return `$${amount.toLocaleString(undefined, { maximumFractionDigits: 6 })}`
}

export function MediaGenerationHistory(props: {
  items: PlaygroundGeneration[]
  total: number
  isLoading: boolean
  isFetching: boolean
  error: string
  deletingId: string | null
  onRefresh: () => void
  onDelete: (id: string) => Promise<void>
  onPreview: (preview: MediaPreview) => void
}) {
  const { t } = useTranslation()
  const [deleteTarget, setDeleteTarget] = useState<PlaygroundGeneration | null>(
    null
  )

  const confirmDelete = async () => {
    if (!deleteTarget) return
    await props.onDelete(deleteTarget.id)
    setDeleteTarget(null)
  }

  return (
    <section className='shrink-0 border-b px-4 py-3'>
      <div className='mb-2 flex items-center justify-between gap-3'>
        <div className='min-w-0'>
          <h2 className='truncate text-sm font-medium'>
            {t('Recent generations')}
          </h2>
          <p className='text-muted-foreground text-xs'>
            {t('{{count}} saved result(s)', { count: props.total })}
          </p>
        </div>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type='button'
                size='icon-sm'
                variant='ghost'
                disabled={props.isFetching}
                onClick={props.onRefresh}
                aria-label={t('Refresh history')}
              />
            }
          >
            <RefreshCw
              className={props.isFetching ? 'size-4 animate-spin' : 'size-4'}
            />
          </TooltipTrigger>
          <TooltipContent>{t('Refresh history')}</TooltipContent>
        </Tooltip>
      </div>

      {props.isLoading ? (
        <div className='text-muted-foreground flex h-28 items-center justify-center gap-2 text-xs'>
          <Loader2 className='size-4 animate-spin' />
          {t('Loading history')}
        </div>
      ) : null}

      {!props.isLoading && props.error ? (
        <div className='text-destructive flex h-28 items-center justify-center gap-2 text-xs'>
          <TriangleAlert className='size-4' />
          <span className='line-clamp-2'>{props.error}</span>
        </div>
      ) : null}

      {!props.isLoading && !props.error && props.items.length === 0 ? (
        <div className='text-muted-foreground flex h-28 items-center justify-center text-xs'>
          {t('No saved generations yet.')}
        </div>
      ) : null}

      {!props.isLoading && !props.error && props.items.length > 0 ? (
        <div className='overflow-x-auto pb-1'>
          <div className='grid w-max auto-cols-[11rem] grid-flow-col gap-2'>
            {props.items.map((item) => {
              const output = item.outputs[0]
              const canPreview = item.status === 'succeeded' && Boolean(output)
              const OperationIcon =
                item.operation === 'video' ? Video : ImageIcon
              let PlaceholderIcon = OperationIcon
              let placeholderClassName = 'size-5'
              if (item.status === 'pending') {
                PlaceholderIcon = Loader2
                placeholderClassName = 'size-5 animate-spin'
              } else if (item.status === 'failed') {
                PlaceholderIcon = TriangleAlert
                placeholderClassName = 'text-destructive size-5'
              }
              return (
                <article
                  key={item.id}
                  className='bg-background group relative h-40 overflow-hidden rounded-md border'
                >
                  <button
                    type='button'
                    className='bg-muted/30 relative block h-20 w-full overflow-hidden border-b text-left disabled:cursor-default'
                    disabled={!canPreview}
                    onClick={() => {
                      if (output) {
                        props.onPreview({
                          source: output,
                          operation: item.operation,
                        })
                      }
                    }}
                    aria-label={t('Open generated result')}
                  >
                    {canPreview && item.operation === 'image' ? (
                      <img
                        src={output}
                        alt={t('Generated image')}
                        className='size-full object-cover'
                        loading='lazy'
                        referrerPolicy='no-referrer'
                      />
                    ) : null}
                    {canPreview && item.operation === 'video' ? (
                      <video
                        src={output}
                        className='size-full object-cover'
                        muted
                        playsInline
                        preload='metadata'
                      />
                    ) : null}
                    {!canPreview ? (
                      <div className='text-muted-foreground flex size-full items-center justify-center'>
                        <PlaceholderIcon className={placeholderClassName} />
                      </div>
                    ) : null}
                    {canPreview ? (
                      <span className='bg-background/85 absolute right-1 bottom-1 flex size-6 items-center justify-center rounded-md border opacity-0 transition-opacity group-focus-within:opacity-100 group-hover:opacity-100'>
                        <Maximize2 className='size-3.5' />
                      </span>
                    ) : null}
                  </button>

                  <div className='space-y-1 px-2 py-1.5'>
                    <div className='flex items-center justify-between gap-1'>
                      <Badge
                        variant={generationStatusVariant(item.status)}
                        className='h-4 max-w-20 truncate px-1 text-[9px]'
                      >
                        {generationStatusLabel(item.status, t)}
                      </Badge>
                      <Tooltip>
                        <TooltipTrigger
                          render={
                            <Button
                              type='button'
                              size='icon-xs'
                              variant='ghost'
                              className='text-muted-foreground hover:text-destructive'
                              disabled={props.deletingId === item.id}
                              onClick={() => setDeleteTarget(item)}
                              aria-label={t('Delete generation')}
                            />
                          }
                        >
                          {props.deletingId === item.id ? (
                            <Loader2 className='size-3 animate-spin' />
                          ) : (
                            <Trash2 className='size-3' />
                          )}
                        </TooltipTrigger>
                        <TooltipContent>
                          {t('Delete generation')}
                        </TooltipContent>
                      </Tooltip>
                    </div>
                    <p
                      className='truncate font-mono text-[10px]'
                      title={item.model}
                    >
                      {item.model}
                    </p>
                    <p
                      className='text-muted-foreground truncate text-[10px]'
                      title={item.prompt}
                    >
                      {item.prompt}
                    </p>
                    <p className='text-muted-foreground flex items-center justify-between gap-1 text-[9px]'>
                      <span>{generationAmount(item.amount)}</span>
                      <span className='flex min-w-0 items-center gap-1'>
                        <Clock3 className='size-2.5' />
                        <span className='truncate'>
                          {generationTime(item.created_at)}
                        </span>
                      </span>
                    </p>
                  </div>
                </article>
              )
            })}
          </div>
        </div>
      ) : null}

      <AlertDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null)
        }}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t('Delete generation?')}</AlertDialogTitle>
            <AlertDialogDescription>
              {t('This saved generation will be removed from your history.')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={props.deletingId !== null}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              variant='destructive'
              disabled={props.deletingId !== null}
              onClick={() => void confirmDelete()}
            >
              {props.deletingId !== null ? t('Deleting...') : t('Delete')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </section>
  )
}
