import { Outlet, useNavigate, useRouter } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { LogOut } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { consoleApi } from '@/api/client'
import type { components } from '@/api/schema'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { Button } from '@/components/ui/button'
import { PortalBrand, PortalNavigation, PortalShell, PortalUser } from '@/components/ui/portal-shell'
import { AccountError } from '@/features/auth/account-shared'
import { AccountProvider, useMerchantAccount } from '@/features/merchant/AccountProvider'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { consoleNavigation } from './navigation'

export function ConsoleLayout({ session }: { session: components['schemas']['SessionIdentity'] }) {
  return <AccountProvider role="console" session={session}><ConsoleContent /></AccountProvider>
}

function ConsoleContent() {
  const { t } = useTranslation(), account = useMerchantAccount()!, navigate = useNavigate(), router = useRouter(), client = useQueryClient(), action = useLocalAction()
  const [open, setOpen] = useState(false), menu = useRef<HTMLButtonElement>(null)
  useEffect(() => router.subscribe('onBeforeNavigate', () => setOpen(false)), [router])
  const close = () => setOpen(false)
  async function logout() {
    await action.run(async () => { await consoleApi.logout(); client.clear(); if (action.active.current) await navigate({ to: '/console/login', search: {}, replace: true }) })
  }
  return <PortalShell title={t('console.navigation')} open={open} onOpenChange={setOpen} menu={menu}
    logo={<PortalBrand to="/console" tag={t('console.manage')} onClose={close} />}
    navigation={<PortalNavigation title={t('console.navigation')} items={consoleNavigation.filter((item) => item.enabled).map((item) => ({ ...item, exact: item.key === 'overview', label: t(`console.nav.${item.key}`) }))} onClose={close} />}
    userBlock={<PortalUser name={account.user.username} role={t('console.admin')} to="/console/account" onClose={close} />}
    controls={<><LanguageSwitch /><ThemeSwitch /><Button variant="ghost" loading={action.busy} disabled={account.busy} onClick={() => void logout()}><LogOut aria-hidden="true" />{t('merchant.logout')}</Button></>}
    problem={action.error != null && <AccountError error={action.error} />}><Outlet /></PortalShell>
}
