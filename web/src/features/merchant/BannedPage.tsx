import { useQuery } from '@tanstack/react-query'
import { useRouteContext } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import { Notice } from '@/components/ui/notice'
import { StatusDot } from '@/components/StatusDot'
import { authErrorKey } from '@/features/auth/errors'

export function BannedPage() {
  const { t, i18n } = useTranslation()
  const { session } = useRouteContext({ from: '/_merchant' })
  const info = useQuery({ queryKey: ['merchant', 'banned', session.user.id], queryFn: api.banned })
  const contact = i18n.resolvedLanguage === 'en' ? info.data?.contact.en : info.data?.contact.zh_CN
  return <section className="flex max-w-xl flex-col items-start gap-5"><StatusDot tone="off">{t('merchant.bannedStatus')}</StatusDot><h1 className="m-0 font-serif text-title font-normal">{t('merchant.bannedTitle')}</h1>{info.isPending && <p role="status">{t('auth.loading')}</p>}{info.error && <Notice tone="error">{t(authErrorKey(info.error))}</Notice>}{info.data && <><p className="m-0 break-words text-text-2">{info.data.reason || t('merchant.bannedDefault')}</p>{contact && <p className="m-0 break-words text-label text-text-2">{contact}</p>}</>}</section>
}
