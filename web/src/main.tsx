import '@fontsource-variable/source-serif-4/wght.css'
import '@fontsource-variable/noto-serif-sc/wght.css'
import '@fontsource-variable/instrument-sans/wght.css'
import '@fontsource-variable/noto-sans-sc/wght.css'
import '@fontsource-variable/jetbrains-mono/wght.css'
import './styles.css'

import { RouterProvider } from '@tanstack/react-router'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { createI18n } from '@/lib/i18n'
import { initTheme } from '@/lib/theme'

initTheme()
const i18n = createI18n()
const queryClient = createQueryClient()
const router = createAppRouter({ queryClient })

const container = document.getElementById('root')
if (!container) {
  throw new Error('Missing #root element')
}

createRoot(container).render(
  <StrictMode>
    <AppProviders i18n={i18n} queryClient={queryClient}>
      <RouterProvider router={router} />
    </AppProviders>
  </StrictMode>,
)
