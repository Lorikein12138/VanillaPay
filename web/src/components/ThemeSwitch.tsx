import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { setTheme, themePreferences, useTheme, type ThemePreference } from '@/lib/theme'
import { useMerchantAccount } from '@/features/merchant/AccountProvider'
import { DropdownMenu } from './ui/dropdown-menu'

const icons: Record<ThemePreference, LucideIcon> = { system: Monitor, light: Sun, dark: Moon }

export function ThemeSwitch() {
  const { t } = useTranslation()
  const theme = useTheme()
  const account = useMerchantAccount()
  const Icon = icons[theme]
  return <DropdownMenu label={t('theme.label')} currentLabel={t(`theme.${theme}`)}
    trigger={<Icon aria-hidden="true" />} disabled={account?.busy} value={theme}
    options={themePreferences.map((value) => {
      const ItemIcon = icons[value]
      return { value, label: t(`theme.${value}`), icon: <ItemIcon aria-hidden="true" className="size-4" /> }
    })}
    onValueChange={(value) => {
      if (account) void account.save({ ...account.preferences, theme: value })
      else setTheme(value)
    }} />
}
