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
import { ImageIcon, MessageSquare, Video } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { PlaygroundChat } from './components/chat/playground-chat'
import { PlaygroundInput } from './components/input/playground-input'
import { MediaPlayground } from './components/media-playground'
import {
  useChatHandler,
  usePlaygroundConversation,
  usePlaygroundOptions,
  usePlaygroundState,
} from './hooks'

type PlaygroundMode = 'text' | 'image' | 'video'

function TextPlayground() {
  const {
    config,
    parameterEnabled,
    messages,
    isLoadingMessages,
    models,
    groups,
    updateMessages,
    setModels,
    setGroups,
    updateConfig,
    clearMessages,
  } = usePlaygroundState()

  const { sendChat, stopGeneration, isGenerating } = useChatHandler({
    config,
    parameterEnabled,
    onMessageUpdate: updateMessages,
  })

  const {
    editingMessageKey,
    handleSendMessage,
    handleRegenerateMessage,
    handleEditMessage,
    handleEditOpenChange,
    applyEdit,
    handleDeleteMessage,
  } = usePlaygroundConversation({
    messages,
    updateMessages,
    sendChat,
  })

  const handleClearMessages = () => {
    handleEditOpenChange(false)
    clearMessages()
  }

  const { isLoadingModels } = usePlaygroundOptions({
    currentGroup: config.group,
    currentModel: config.model,
    setGroups,
    setModels,
    updateConfig,
  })

  return (
    <div className='relative flex size-full min-h-0 flex-col overflow-hidden'>
      {/* Full-width scroll container: scrolling works even over side whitespace */}
      <div className='flex min-h-0 flex-1 flex-col overflow-hidden'>
        <PlaygroundChat
          messages={messages}
          isLoadingMessages={isLoadingMessages}
          onRegenerateMessage={handleRegenerateMessage}
          onEditMessage={handleEditMessage}
          onDeleteMessage={handleDeleteMessage}
          onSelectPrompt={handleSendMessage}
          isGenerating={isGenerating}
          editingKey={editingMessageKey}
          onCancelEdit={handleEditOpenChange}
          onSaveEdit={(newContent) => applyEdit(newContent, false)}
          onSaveEditAndSubmit={(newContent) => applyEdit(newContent, true)}
        />
      </div>

      {/* Input area: center content and constrain to the same container width */}
      <div className='mx-auto w-full max-w-4xl'>
        <PlaygroundInput
          disabled={isGenerating}
          groups={groups}
          groupValue={config.group}
          isGenerating={isGenerating}
          isModelLoading={isLoadingModels}
          modelValue={config.model}
          models={models}
          onGroupChange={(value) => updateConfig('group', value)}
          onClearMessages={handleClearMessages}
          onModelChange={(value) => updateConfig('model', value)}
          onStop={stopGeneration}
          onSubmit={handleSendMessage}
          hasMessages={messages.length > 0}
        />
      </div>
    </div>
  )
}

export function Playground() {
  const { t } = useTranslation()
  const [mode, setMode] = useState<PlaygroundMode>('text')

  return (
    <div className='flex size-full min-h-0 flex-col overflow-hidden'>
      <header className='flex shrink-0 items-center justify-between gap-3 border-b px-4 py-2'>
        <div className='min-w-0'>
          <h1 className='truncate text-sm font-medium'>{t('Playground')}</h1>
          <p className='text-muted-foreground truncate text-xs'>
            {t('Test published text, image, and video model contracts.')}
          </p>
        </div>
        <Tabs
          value={mode}
          onValueChange={(value) => setMode(value as PlaygroundMode)}
        >
          <TabsList className='h-8'>
            <TabsTrigger value='text' className='h-7 px-2.5 text-xs'>
              <MessageSquare className='size-3.5' />
              {t('Text')}
            </TabsTrigger>
            <TabsTrigger value='image' className='h-7 px-2.5 text-xs'>
              <ImageIcon className='size-3.5' />
              {t('Image')}
            </TabsTrigger>
            <TabsTrigger value='video' className='h-7 px-2.5 text-xs'>
              <Video className='size-3.5' />
              {t('Video')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </header>

      <div className='flex min-h-0 flex-1 flex-col overflow-hidden'>
        {mode === 'text' && <TextPlayground />}
        {mode === 'image' && <MediaPlayground operation='image.generate' />}
        {mode === 'video' && <MediaPlayground operation='video.generate' />}
      </div>
    </div>
  )
}
