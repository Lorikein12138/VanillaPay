import { useTranslation } from 'react-i18next'
import { changeLanguage, languages } from '@/lib/i18n'
import { useMerchantAccount } from '@/features/merchant/AccountProvider'
import { DropdownMenu } from './ui/dropdown-menu'

export function LanguageSwitch() {
  const { t, i18n } = useTranslation()
  const account = useMerchantAccount()
  const value = i18n.resolvedLanguage === 'en' ? 'en' : 'zh-CN'
  return <DropdownMenu label={t('language.label')} currentLabel={t(`language.${value}`)}
    trigger={<span aria-hidden="true">{value === 'en' ? 'EN' : '中'}</span>}
    disabled={account?.busy} value={value}
    options={languages.map((language) => ({ value: language, label: t(`language.${language}`), lang: language }))}
    onValueChange={(locale) => {
      if (account) void account.save({ ...account.preferences, locale })
      else void changeLanguage(i18n, locale)
    }} />
}
