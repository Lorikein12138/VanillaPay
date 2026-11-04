import { Link, Outlet, useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { LogOut } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import type { components } from '@/api/schema'
import { FormError } from '@/components/FormError'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { LogoMark } from '@/components/Logo'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { authErrorKey } from '@/features/auth/errors'
import { merchantNavigation } from '@/features/merchant/navigation'

export function MerchantLayout({ session }: { session: components['schemas']['SessionIdentity'] }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [busy, setBusy] = useState(false)
  const [problem, setProblem] = useState<unknown>()
  async function logout() {
    if (busy) return
    setBusy(true)
    setProblem(undefined)
    try { await api.logout(); queryClient.clear(); await navigate({ to: '/login', search: {}, replace: true }) }
    catch (error) { setProblem(error) }
    finally { setBusy(false) }
  }
  return (
    <div className="grid min-h-dvh min-w-0 grid-rows-[auto_1fr] lg:grid-cols-[224px_1fr] lg:grid-rows-1">
      <aside className="flex min-w-0 flex-col gap-7 bg-sidebar px-4 py-6 lg:min-h-dvh">
        <Link to="/dashboard" className="flex min-h-11 items-center gap-2.5 rounded-control px-2"><LogoMark /><span className="font-serif text-xl font-medium">{t('common.brand')}</span></Link>
        {!session.user.banned && <nav aria-label={t('merchant.navigation')} className="flex flex-col gap-1">{merchantNavigation.filter((item) => item.enabled).map((item) => <Link key={item.key} to={item.path} activeOptions={{ exact: true }} activeProps={{ 'aria-current': 'page', className: 'bg-hover text-text' }} inactiveProps={{ className: 'text-text-2' }} className="flex h-10 items-center gap-3 rounded-control px-3 text-label transition-colors hover:bg-hover hover:text-text"><item.icon className="size-4 shrink-0" strokeWidth={1.6} aria-hidden="true" />{t(`merchant.nav.${item.key}`)}</Link>)}</nav>}
        <div className="mt-auto flex min-w-0 items-center gap-3 border-t-[0.5px] border-border px-2 pt-4"><span aria-hidden="true" className="grid size-9 shrink-0 place-items-center rounded-full bg-oat text-label">{session.user.username.slice(0, 1)}</span><div className="min-w-0"><div className="truncate text-label">{session.user.username}</div><div className="truncate font-mono text-caption text-text-3">{session.user.pid}</div></div></div>
      </aside>
      <div className="min-w-0">
        <header className="flex flex-wrap items-center justify-end gap-2 px-4 py-4 sm:px-8"><LanguageSwitch /><ThemeSwitch /><button type="button" disabled={busy} onClick={() => { void logout() }} className="inline-flex min-h-11 items-center gap-2 rounded-control px-3 text-label text-text-2 hover:bg-hover disabled:opacity-50"><LogOut className="size-4" strokeWidth={1.6} aria-hidden="true" />{t(busy ? 'auth.submitting' : 'merchant.logout')}</button></header>
        {problem != null && <div className="px-4 sm:px-8"><FormError>{t(authErrorKey(problem))}</FormError></div>}
        <main id="main" className="min-w-0 px-4 py-8 sm:px-8 lg:px-12 lg:py-10"><Outlet /></main>
      </div>
    </div>
  )
}
