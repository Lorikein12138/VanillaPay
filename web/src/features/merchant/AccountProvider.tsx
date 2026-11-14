import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useRouter, useRouterState } from '@tanstack/react-router'
import { createContext, useContext, useEffect, useRef, useState, useSyncExternalStore, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { api, consoleApi } from '@/api/client'
import { sessionCookie, subscribeToSessionCookies } from '@/api/session-cookie'
import type { components } from '@/api/schema'
import { clearPortalSession, isSessionRejected, sessionQuery } from '@/app/portal-session'
import { AccountError } from '@/features/auth/account-shared'
import { changeLanguage } from '@/lib/i18n'
import { consoleSafePath, safePath } from '@/lib/redirect'
import { setTheme } from '@/lib/theme'
import { useLocalAction } from './CollectionParts'

import { restrictedMerchantPath } from './navigation'

type Schema = components['schemas']
type AccountContextValue = { user: Schema['AuthUserDTO']; preferences: Schema['Preferences']; busy: boolean; save: (value: Schema['Preferences']) => Promise<void>; refresh: () => Promise<void> }
const AccountContext = createContext<AccountContextValue | null>(null)
export function useMerchantAccount() { return useContext(AccountContext) }
export function accountPreferences(user: Schema['AuthUserDTO']): Schema['Preferences'] { return { locale: user.locale === 'en' ? 'en' : 'zh-CN', theme: user.theme === 'light' || user.theme === 'dark' ? user.theme : 'system' } }

export function AccountProvider({ session, children, role = 'merchant' }: { role?: 'merchant' | 'console'; session: Schema['SessionIdentity']; children: ReactNode }) {
  const client = useQueryClient(), router = useRouter(), { i18n } = useTranslation(), action = useLocalAction()
  const accountApi = role === 'console' ? consoleApi : api
  const options = sessionQuery(role)
  const key = options.queryKey
  const identity = useQuery(options)
  const cookie = useSyncExternalStore(subscribeToSessionCookies, () => sessionCookie(role))
  const hasIdentity = !!identity.data
  const user = identity.data?.user ?? session.user
  const preferences = accountPreferences(user)
  const location = useRouterState({ select: (state) => state.resolvedLocation ?? state.location })
  const restrictedPath = restrictedMerchantPath(user)
  const wrongMerchantPath = role === 'merchant' && (restrictedPath
    ? location.pathname !== restrictedPath
    : location.pathname === '/banned' || location.pathname === '/registration-review')
  const changedAccount = user.id !== session.user.id
  const changedName = user.username !== session.user.username
  const changedCookie = hasIdentity && identity.data.cookie !== cookie
  const rejected = !changedCookie && isSessionRejected(identity.error)
  const changedSession = hasIdentity && identity.data.csrf_token !== session.csrf_token
  const saving = useRef(false), [busy, setBusy] = useState(false)
  useEffect(() => {
    if (rejected) {
      accountApi.forgetSession()
      clearPortalSession(client, role)
      if (role === 'console') {
        void router.navigate({ to: '/console/login', search: { redirect: consoleSafePath(location.href) }, replace: true })
      } else {
        void router.navigate({ to: '/login', search: { redirect: safePath(location.href) }, replace: true })
      }
    } else if (wrongMerchantPath || changedAccount || changedName || changedCookie || changedSession) {
      void router.invalidate()
    }
  }, [accountApi, wrongMerchantPath, changedAccount, changedName, changedCookie, changedSession, client, location.href, rejected, role, router])
  useEffect(() => {
    if (hasIdentity && !rejected && !changedCookie) {
      setTheme(preferences.theme)
      void changeLanguage(i18n, preferences.locale)
    }
  }, [changedCookie, hasIdentity, i18n, preferences.locale, preferences.theme, rejected])
  async function apply(value: Schema['Preferences']) { setTheme(value.theme); await changeLanguage(i18n, value.locale) }
  async function refresh() {
    await client.cancelQueries({ queryKey: key, exact: true })
    await client.fetchQuery({ ...options, staleTime: 0 })
    if (action.active.current) await router.invalidate()
  }
  async function save(value: Schema['Preferences']) {
    if (saving.current) return
    saving.current = true; setBusy(true)
    await action.run(async () => {
      await client.cancelQueries({ queryKey: key, exact: true })
      await apply(value)
      try {
        const result = await accountApi.preferences(value)
        if (action.active.current) {
          await client.cancelQueries({ queryKey: key, exact: true })
          const updated = client.setQueryData(key, (old) => old?.user.id === user.id ? { ...old, user: { ...old.user, ...result } } : old)
          if (updated?.user.id === user.id) await apply(result)
        }
      } catch (error) { if (action.active.current) await apply(preferences); throw error }
    })
    saving.current = false
    if (action.active.current) setBusy(false)
  }
  if (!identity.data || rejected || wrongMerchantPath || changedAccount || changedCookie || changedSession) return null
  return <AccountContext.Provider value={{ user, preferences, busy, save, refresh }}>{action.error != null && <div className="px-4 pt-4 sm:px-8"><AccountError error={action.error} /></div>}{children}</AccountContext.Provider>
}
