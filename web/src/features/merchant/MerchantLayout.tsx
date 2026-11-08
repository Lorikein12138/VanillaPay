import { Link, Outlet, useRouter, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { LogOut } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import type { components } from '@/api/schema'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { Button } from '@/components/ui/button'
import { PortalBrand, PortalShell } from '@/components/ui/portal-shell'
import { Notice } from '@/components/ui/notice'
import { authErrorKey } from '@/features/auth/errors'
import { merchantNavigation } from '@/features/merchant/navigation'
import { AccountProvider, useMerchantAccount } from './AccountProvider'
import { useNotificationSummary } from './notification-data'
import { PopupAnnouncements, UnreadBadge } from './NotificationsPage'

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
  const navigation = !session.user.banned && <nav aria-label={t('merchant.navigation')} className="flex flex-col gap-1">{merchantNavigation.filter((item) => item.enabled).map((item) => <Link key={item.key} to={item.path} onClick={() => setOpen(false)} activeOptions={{ exact: true }} activeProps={{ 'aria-current': 'page', className: 'bg-hover text-text' }} inactiveProps={{ className: 'text-text-2' }} className="flex min-h-11 items-center gap-3 rounded-control px-3 text-label transition-colors hover:bg-hover hover:text-text"><item.icon className="size-4 shrink-0" strokeWidth={1.6} aria-hidden="true" />{t(`merchant.nav.${item.key}`)}{item.key === 'notifications' && <UnreadBadge count={notifications.data?.unread_count ?? 0} />}</Link>)}</nav>
  const user = <><span aria-hidden="true" className="grid size-9 shrink-0 place-items-center rounded-full bg-oat text-label">{session.user.username.slice(0, 1)}</span><span className="min-w-0"><span className="block truncate text-label">{session.user.username}</span><span className="block truncate font-mono text-caption text-text-3">{session.user.pid}</span></span></>
  const userClass = 'flex min-h-11 min-w-0 items-center gap-3 rounded-control px-2 py-3'
  const userBlock = <div className="mt-auto border-t-[0.5px] border-border pt-4">{session.user.banned ? <div className={userClass}>{user}</div> : <Link to="/account" aria-label={session.user.username} onClick={() => setOpen(false)} className={`${userClass} hover:bg-hover`}>{user}</Link>}</div>
  const controls = <><LanguageSwitch /><ThemeSwitch /><Button variant="ghost" loading={busy} disabled={account.busy} onClick={() => { void logout() }}><LogOut aria-hidden="true" strokeWidth={1.6} />{t(busy ? 'auth.submitting' : 'merchant.logout')}</Button></>
  return <><PortalShell logo={logo} navigation={navigation} userBlock={userBlock} controls={controls} problem={problem != null && <Notice tone="error" onDismiss={() => setProblem(undefined)}>{t(authErrorKey(problem))}</Notice>} open={open} onOpenChange={setOpen} menu={menu} title={t('merchant.navigation')}><Outlet /></PortalShell>
    {!session.user.banned && <PopupAnnouncements key={session.user.username} name={session.user.username} items={notifications.data?.popup_announcements ?? []} />}
  </>
}
