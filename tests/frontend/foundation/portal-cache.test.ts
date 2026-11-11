import { afterEach, describe, expect, it, vi } from 'vitest'
import { queryOptions } from '@tanstack/react-query'
import { api, consoleApi } from '@/api/client'
import type { components } from '@/api/schema'
import { preparePortalRoute } from '@/app/portal-preload'
import { createQueryClient } from '@/app/providers'
import { clearPortalSession, ensurePortalSession, sessionQuery, setPortalSession } from '@/app/portal-session'

type Session = components['schemas']['SessionIdentity']
function identity(id: string): Session {
  return {
    user: {
      id, username: `user_${id}`, email: `user_${id}@example.test`, role: 'merchant',
      banned: false, banned_at: null, ban_reason: null, totp_enabled: false,
      locale: 'zh-CN', theme: 'light', created_at: new Date().toISOString(),
    },
    csrf_token: crypto.randomUUID(),
    expires_at: new Date(Date.now() + 3_600_000).toISOString(),
  }
}

afterEach(() => {
  vi.restoreAllMocks()
  for (const role of ['merchant', 'console']) document.cookie = `vp_csrf_${role}=; Max-Age=0; Path=/`
})

describe('portal route data', () => {
  it('uses stale cached data immediately while its refresh is still in flight', async () => {
    const client = createQueryClient()
    let finish!: (result: string) => void
    const queryFn = vi.fn(() => new Promise<string>((resolve) => { finish = resolve }))
    const options = queryOptions({ queryKey: ['merchant', 'orders'], queryFn, staleTime: 1000 })
    client.setQueryData(options.queryKey, 'cached', { updatedAt: Date.now() - 2000 })
    await preparePortalRoute(client, () => true, 'enter', ({ query }) => [query(options)])
    expect(queryFn).toHaveBeenCalledOnce()
    expect(client.getQueryData(options.queryKey)).toBe('cached')
    expect(client.isFetching()).toBe(1)
    finish('fresh')
    await expect(client.fetchQuery(options)).resolves.toBe('fresh')
    client.clear()
  })

  it('shares cold prefetch with navigation and leaves same-page changes to the mounted observer', async () => {
    const client = createQueryClient()
    let finish!: (result: string) => void
    const queryFn = vi.fn(() => new Promise<string>((resolve) => { finish = resolve }))
    const options = queryOptions({ queryKey: ['console', 'orders'], queryFn })
    const prefetch = (queries: { query: typeof client.ensureQueryData }) => [queries.query(options)]
    expect(preparePortalRoute(client, () => true, 'stay', prefetch)).toBeUndefined()
    expect(queryFn).not.toHaveBeenCalled()
    expect(preparePortalRoute(client, () => false, 'enter', prefetch)).toBeUndefined()
    const preload = preparePortalRoute(client, () => true, 'preload', prefetch)
    const loaded = vi.fn()
    const enter = preparePortalRoute(client, () => true, 'enter', prefetch)?.then(loaded)
    await Promise.resolve()
    expect(queryFn).toHaveBeenCalledOnce()
    expect(loaded).not.toHaveBeenCalled()
    finish('loaded')
    await Promise.all([preload, enter])
    expect(loaded).toHaveBeenCalledOnce()
    client.clear()
  })
})

describe('portal sessions', () => {
  it('deduplicates guard requests, keeps each role independent and accepts a login result without another request', async () => {
    const client = createQueryClient()
    const merchant = identity('1'), admin = { ...identity('2'), user: { ...identity('2').user, role: 'admin' as const } }
    const me = vi.spyOn(api, 'me').mockResolvedValue(merchant)
    const consoleMe = vi.spyOn(consoleApi, 'me').mockResolvedValue(admin)
    await expect(Promise.all([ensurePortalSession(client, 'merchant'), ensurePortalSession(client, 'merchant')])).resolves.toEqual([{ ...merchant, cookie: '' }, { ...merchant, cookie: '' }])
    expect(me).toHaveBeenCalledOnce()
    document.cookie = `vp_csrf_console=${admin.csrf_token}; Path=/`
    setPortalSession(client, 'console', admin)
    await expect(ensurePortalSession(client, 'console')).resolves.toMatchObject(admin)
    expect(consoleMe).not.toHaveBeenCalled()
    await expect(ensurePortalSession(client, 'merchant')).resolves.toMatchObject(merchant)
    expect(me).toHaveBeenCalledOnce()
    client.clear()
  })

  it('refreshes an expired identity and clears only its role data', async () => {
    const client = createQueryClient(), merchant = identity('1')
    const me = vi.spyOn(api, 'me').mockResolvedValue(merchant)
    setPortalSession(client, 'merchant', { ...merchant, expires_at: new Date(0).toISOString() })
    const orders = ['merchant', 'orders'], plans = ['console', 'plans']
    client.setQueryData(orders, 'old')
    client.setQueryData(plans, 'keep')
    await expect(ensurePortalSession(client, 'merchant')).resolves.toMatchObject(merchant)
    expect(me).toHaveBeenCalledOnce()
    expect(client.getQueryData(orders)).toBeUndefined()
    expect(client.getQueryData(plans)).toBe('keep')
    client.clear()
  })

  it('does not restore a cleared session or page from late requests', async () => {
    const client = createQueryClient(), old = identity('1'), current = identity('2')
    let finishSession!: (session: Session) => void
    let finishOrders!: (orders: string) => void
    vi.spyOn(api, 'me').mockImplementation(() => new Promise((resolve) => { finishSession = resolve }))
    const session = client.fetchQuery(sessionQuery('merchant')).catch((error: unknown) => error)
    const orders = ['merchant', 'orders']
    const data = client.prefetchQuery({ queryKey: orders, queryFn: () => new Promise((resolve) => { finishOrders = resolve }) })
    clearPortalSession(client, 'merchant')
    document.cookie = `vp_csrf_merchant=${current.csrf_token}; Path=/`
    setPortalSession(client, 'merchant', current)
    finishSession(old)
    finishOrders('old')
    await Promise.all([session, data])
    expect(client.getQueryData(orders)).toBeUndefined()
    await expect(ensurePortalSession(client, 'merchant')).resolves.toMatchObject(current)
    client.clear()
  })

  it('evicts cached data when a refreshed cookie belongs to another account', async () => {
    const client = createQueryClient()
    const first = identity('1'), next = identity('2')
    setPortalSession(client, 'merchant', { ...first, user: { ...first.user, username: 'session' } })
    const orders = ['merchant', 'session', 'orders']
    client.setQueryData(orders, 'private')
    vi.spyOn(api, 'me').mockResolvedValue(next)
    await expect(client.fetchQuery({ ...sessionQuery('merchant'), staleTime: 0 })).resolves.toMatchObject(next)
    expect(client.getQueryData(orders)).toBeUndefined()
    await expect(ensurePortalSession(client, 'merchant')).resolves.toMatchObject(next)
    client.clear()
  })
})
