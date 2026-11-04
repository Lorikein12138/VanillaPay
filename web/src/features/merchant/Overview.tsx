import { useTranslation } from 'react-i18next'

export function Overview({ name }: { name: string }) {
  const { t, i18n } = useTranslation()
  const date = new Intl.DateTimeFormat(i18n.resolvedLanguage, { dateStyle: 'long' }).format(new Date())
  return <><h1 className="m-0 font-serif text-title font-normal">{t('merchant.nav.overview')}</h1><p className="mt-2 text-label text-text-2">{name} · {date}</p></>
}
