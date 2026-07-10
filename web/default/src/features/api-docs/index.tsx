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
  ApiReferenceReact,
  type AnyApiReferenceConfiguration,
} from '@scalar/api-reference-react'
import { useTheme } from 'next-themes'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'

import '@scalar/api-reference-react/style.css'
import './api-docs.css'

export function ApiDocs() {
  const { i18n } = useTranslation()
  const { resolvedTheme } = useTheme()

  const configuration = useMemo<AnyApiReferenceConfiguration>(() => {
    const language = i18n.resolvedLanguage || i18n.language || 'en'
    const locale = language.startsWith('zh') ? 'zh-CN' : language
    const isDark =
      resolvedTheme === 'dark' ||
      document.documentElement.classList.contains('dark')

    return {
      url: '/openapi/relay.json',
      _integration: 'react',
      agent: { disabled: true, hideAddApi: true },
      baseServerURL: window.location.origin,
      defaultOpenAllTags: false,
      forceDarkModeState: isDark ? 'dark' : 'light',
      hideClientButton: true,
      hideDarkModeToggle: true,
      layout: 'modern',
      localization: { locale },
      mcp: { disabled: true },
      persistAuth: false,
      servers: [{ url: window.location.origin }],
      showDeveloperTools: 'never',
      telemetry: false,
      theme: 'default',
      withDefaultFonts: false,
    }
  }, [i18n.language, i18n.resolvedLanguage, resolvedTheme])

  return (
    <PublicLayout showMainContainer={false}>
      <main className='api-docs-shell'>
        <ApiReferenceReact configuration={configuration} />
      </main>
    </PublicLayout>
  )
}
