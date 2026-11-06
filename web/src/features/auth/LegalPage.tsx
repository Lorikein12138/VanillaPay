import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import { TopBar } from '@/components/TopBar'
import { AccountError, type AccountSchema } from './account-shared'

export function LegalPage({ kind }: { kind: AccountSchema['LegalVersion']['kind'] }) {
  const { i18n } = useTranslation()
  const legal = useQuery({ queryKey: ['public-legal', kind], queryFn: () => api.legal(kind), retry: false })
  const content = legal.data?.content
  return <><TopBar /><main id="main" className="mx-auto max-w-3xl px-4 py-12 sm:px-8"><h1 className="mb-6 font-serif text-4xl">{kind === 'terms' ? '服务条款' : '隐私政策'}</h1>{legal.isPending ? <p role="status">正在加载…</p> : legal.error instanceof APIError && legal.error.status === 404 ? <p role="status">暂未发布</p> : legal.error ? <AccountError error={legal.error} /> : content && <><p className="mb-6 text-caption text-text-3">版本 {legal.data.revision} · {legal.data.updated_at}</p><div className="whitespace-pre-wrap break-words text-body leading-loose">{i18n.resolvedLanguage === 'en' ? content.en || content.zh_CN : content.zh_CN || content.en}</div></>}</main></>
}
