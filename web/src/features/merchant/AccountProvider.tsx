import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useRouter } from '@tanstack/react-router'
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import type { components } from '@/api/schema'
import { AccountError } from '@/features/auth/account-shared'
import { changeLanguage } from '@/lib/i18n'
import { setTheme } from '@/lib/theme'
import { useLocalAction } from './CollectionParts'

type Schema = components['schemas']
type AccountContextValue = { user: Schema['AuthUserDTO']; preferences: Schema['Preferences']; busy: boolean; save: (value: Schema['Preferences']) => Promise<void>; refresh: () => Promise<void> }
const AccountContext = createContext<AccountContextValue | null>(null)
export function useMerchantAccount() { return useContext(AccountContext) }
export function accountPreferences(user: Schema['AuthUserDTO']): Schema['Preferences'] { return { locale: user.locale === 'en' ? 'en' : 'zh-CN', theme: user.theme === 'light' || user.theme === 'dark' ? user.theme : 'system' } }

export function AccountProvider({ session, children }: { session: Schema['SessionIdentity']; children: ReactNode }) {
  const client = useQueryClient(), router = useRouter(), { i18n } = useTranslation(), action = useLocalAction()
  const key = ['merchant', 'identity', session.user.id] as const
  const identity = useQuery({ queryKey: key, queryFn: () => api.me(), initialData: session, staleTime: 30000, gcTime: 0 })
  const user = identity.data.user, preferences = accountPreferences(user)
  const saving = useRef(false), [busy, setBusy] = useState(false)
  useEffect(() => { setTheme(preferences.theme); void changeLanguage(i18n, preferences.locale) }, [i18n, preferences.locale, preferences.theme])
  async function apply(value: Schema['Preferences']) { setTheme(value.theme); await changeLanguage(i18n, value.locale) }
  async function refresh() {
    await client.cancelQueries({ queryKey: key })
    const result = await api.me()
    if (action.active.current) { client.setQueryData(key, result); await router.invalidate() }
  }
  async function save(value: Schema['Preferences']) {
    if (saving.current) return
    saving.current = true; setBusy(true)
    await action.run(async () => {
      await client.cancelQueries({ queryKey: key })
      await apply(value)
      try {
        const result = await api.preferences(value)
        if (action.active.current) {
          client.setQueryData<Schema['SessionIdentity']>(key, (old) => old && ({ ...old, user: { ...old.user, ...result } }))
          await apply(result)
        }
      } catch (error) { if (action.active.current) await apply(preferences); throw error }
    })
    saving.current = false
    if (action.active.current) setBusy(false)
  }
  return <AccountContext.Provider value={{ user, preferences, busy, save, refresh }}>{action.error != null && <div className="px-4 pt-4 sm:px-8"><AccountError error={action.error} /></div>}{action.error == null && identity.error && <div className="px-4 pt-4 sm:px-8"><AccountError error={identity.error} /></div>}{children}</AccountContext.Provider>
}
