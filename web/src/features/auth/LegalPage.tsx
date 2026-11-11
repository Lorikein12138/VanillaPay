import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import { PublicFooter } from '@/components/PublicFooter'
import { Notice } from '@/components/ui/notice'
import { TopBar } from '@/components/TopBar'
import { AccountError, type AccountSchema } from './account-shared'

export function LegalPage({ kind }: { kind: AccountSchema['LegalVersion']['kind'] }) {
  const { t, i18n } = useTranslation()
  const legal = useQuery({ queryKey: ['public-legal', kind], queryFn: () => api.legal(kind), retry: false })
  const content = legal.data?.content
  return (
    <div className="flex min-h-dvh flex-col">
      <TopBar />
      <main id="main" className="mx-auto w-full max-w-[720px] flex-1 px-4 pt-14 pb-20 sm:px-8">
        <h1 className="mb-3 font-serif text-[2.25rem] leading-[1.15] font-normal tracking-[-0.015em] sm:text-[2.75rem]">{t(kind === 'terms' ? 'polish.termsOfService' : 'polish.privacyPolicy')}</h1>
        {legal.isPending ? <p role="status">{t('polish.loading')}</p>
          : legal.error instanceof APIError && legal.error.status === 404 ? <Notice tone="info">{t('polish.notPublishedYet')}</Notice>
          : legal.error ? <AccountError error={legal.error} />
          : content && (
            <>
              <p className="mb-10 text-label text-text-3">
                {t('polish.legalRevision', { revision: legal.data.revision })} · {legal.data.updated_at}
              </p>
              <div className="whitespace-pre-wrap break-words font-serif text-[1.0625rem] leading-[1.75] text-text">
                {i18n.resolvedLanguage === 'en' ? content.en || content.zh_CN : content.zh_CN || content.en}
              </div>
            </>
          )}
      </main>
      <PublicFooter />
    </div>
  )
}
