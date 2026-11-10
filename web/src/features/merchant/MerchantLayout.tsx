import { usePreloadPortalCode } from '@/app/portal-preload'
import { Outlet, useRouter, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { LogOut } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import type { components } from '@/api/schema'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { Button } from '@/components/ui/button'
import { PortalBrand, PortalNavigation, PortalShell, PortalUser } from '@/components/ui/portal-shell'
import { Notice } from '@/components/ui/notice'
import { authErrorKey } from '@/features/auth/errors'
import { merchantNavigation } from '@/features/merchant/navigation'
import { AccountProvider, useMerchantAccount } from './AccountProvider'
import { useNotificationSummary } from './notification-data'
import { PopupAnnouncements, UnreadBadge } from './PopupAnnouncements'

const idleRoutes = [...merchantNavigation, { path: '/account', enabled: true }]

export function MerchantLayout({ session }: { session: components['schemas']['SessionIdentity'] }) {
  return <AccountProvider session={session}><MerchantContent /></AccountProvider>
}
function MerchantContent() {
  const account = useMerchantAccount()!
  const session = { user: account.user }
  const { t } = useTranslation()
  const navigate = useNavigate(), router = useRouter()
  const queryClient = useQueryClient()
  const [busy, setBusy] = useState(false), [open, setOpen] = useState(false)
  const [problem, setProblem] = useState<unknown>()
  const menu = useRef<HTMLButtonElement>(null)
  usePreloadPortalCode(idleRoutes, !session.user.banned)
  const notifications = useNotificationSummary(session.user.username, !session.user.banned)
  useEffect(() => router.subscribe('onBeforeNavigate', () => setOpen(false)), [router])
  async function logout() {
    if (busy) return
    setBusy(true)
    setProblem(undefined)
    try { await api.logout(); queryClient.clear(); await navigate({ to: '/login', search: {}, replace: true }) }
    catch (error) { setProblem(error) }
    finally { setBusy(false) }
  }
  const logo = <PortalBrand to="/dashboard" onClose={() => setOpen(false)} />
  const navigation = !session.user.banned && <PortalNavigation title={t('merchant.navigation')} onClose={() => setOpen(false)}
    items={merchantNavigation.filter((item) => item.enabled).map((item) => ({
      ...item, label: t(`merchant.nav.${item.key}`), badge: item.key === 'notifications' ? <UnreadBadge count={notifications.data?.unread_count ?? 0} /> : undefined,
    }))} />
  const userBlock = <PortalUser name={session.user.username} role={session.user.pid ?? ''} mono
    to={session.user.banned ? undefined : '/account'} onClose={() => setOpen(false)} />
  const controls = <><LanguageSwitch /><ThemeSwitch /><Button variant="ghost" loading={busy} disabled={account.busy} onClick={() => { void logout() }}><LogOut aria-hidden="true" strokeWidth={1.6} />{t(busy ? 'auth.submitting' : 'merchant.logout')}</Button></>
  return <><PortalShell logo={logo} navigation={navigation} userBlock={userBlock} controls={controls} problem={problem != null && <Notice tone="error" onDismiss={() => setProblem(undefined)}>{t(authErrorKey(problem))}</Notice>} open={open} onOpenChange={setOpen} menu={menu} title={t('merchant.navigation')}><Outlet /></PortalShell>
    {!session.user.banned && <PopupAnnouncements key={session.user.username} name={session.user.username} items={notifications.data?.popup_announcements ?? []} />}
  </>
}
