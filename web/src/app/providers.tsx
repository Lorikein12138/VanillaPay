import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { i18n as I18n } from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider } from 'react-i18next'

export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
      },
    },
  })
}

export function AppProviders({
  i18n,
  queryClient,
  children,
}: {
  i18n: I18n
  queryClient: QueryClient
  children: ReactNode
}) {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    </I18nextProvider>
  )
}
