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
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'

import type { PlaygroundMediaOperation } from '../types'

export type MediaPreview = {
  source: string
  operation: PlaygroundMediaOperation
}

export function MediaPreviewDialog(props: {
  preview: MediaPreview | null
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation()
  const isVideo = props.preview?.operation === 'video'

  return (
    <Dialog
      open={props.preview !== null}
      onOpenChange={props.onOpenChange}
      title={isVideo ? t('Generated video') : t('Generated image')}
      description={t('Generated result preview')}
      contentClassName='h-[90vh] max-h-[90vh] w-[90vw] max-w-[90vw] p-3 sm:max-w-[90vw] sm:p-4'
      contentHeight='calc(90vh - 6rem)'
      bodyClassName='flex h-full min-h-0 items-center justify-center overflow-hidden p-0'
      showCloseButton
    >
      {props.preview && isVideo ? (
        <video
          key={props.preview.source}
          src={props.preview.source}
          className='max-h-full max-w-full object-contain'
          controls
          autoPlay
          playsInline
          aria-label={t('Generated video')}
        />
      ) : null}
      {props.preview && !isVideo ? (
        <img
          src={props.preview.source}
          alt={t('Generated image')}
          className='max-h-full max-w-full object-contain'
          referrerPolicy='no-referrer'
        />
      ) : null}
    </Dialog>
  )
}
