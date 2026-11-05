import type { components } from '@/api/schema'

type Schema = components['schemas']
export class APIError extends Error {
  constructor(readonly status: number, readonly problem?: Schema['ErrorEnvelope']['error']) {
    super(problem?.code ?? 'REQUEST_FAILED')
  }
}

let csrfToken = ''
let unauthorized: (() => void) | undefined

export function onUnauthorized(handler: () => void) {
  unauthorized = handler
}

function remember<T extends { csrf_token: string }>(session: T): T {
  csrfToken = session.csrf_token
  return session
}

async function request<T>(path: string, body?: unknown, protectedRequest = true): Promise<T> {
  let response: Response
  try {
    response = await fetch(path, {
      method: body === undefined ? 'GET' : 'POST',
      credentials: 'include',
      headers: {
        Accept: 'application/json',
        ...(body === undefined ? {} : { 'Content-Type': 'application/json' }),
        ...(body !== undefined && protectedRequest && csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
      },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    })
  } catch {
    throw new APIError(0)
  }
  const envelope = await response.json().catch(() => null) as Schema['ErrorEnvelope'] | (Schema['SuccessEnvelope'] & { data: T }) | null
  if (!response.ok) {
    if (response.status === 401 && protectedRequest) {
      csrfToken = ''
      unauthorized?.()
    }
    throw new APIError(response.status, envelope && 'error' in envelope && envelope.error ? envelope.error : undefined)
  }
  if (!envelope || !('data' in envelope)) throw new APIError(502)
  return envelope.data
}

export const api = {
  config: () => request<Schema['PublicConfig']>('/api/v1/public/config', undefined, false),
  async login(value: Schema['AuthLoginRequest']) {
    const result = await request<Schema['AuthSessionResult'] | Schema['AuthLoginResult']>('/api/v1/auth/login', value, false)
    return 'requires_2fa' in result ? result : remember(result)
  },
  async secondFactor(value: Schema['AuthFactorRequest']) {
    return remember(await request<Schema['AuthSessionResult']>('/api/v1/auth/second-factor', value, false))
  },
  async me(protectedRequest = true) {
    return remember(await request<Schema['SessionIdentity']>('/api/v1/merchant/me', undefined, protectedRequest))
  },
  banned: () => request<Schema['BannedInfo']>('/api/v1/merchant/banned'),
  stats: (range: { start: string; end: string; timezone: string }) => request<Schema['PortalStatsDTO']>(`/api/v1/merchant/stats?${new URLSearchParams(range)}`),
  device: () => request<Schema['DeviceDeviceDTO']>('/api/v1/merchant/device'),
  orders: (limit: number) => request<Schema['OrdersPage']>(`/api/v1/merchant/orders?${new URLSearchParams({ limit: String(limit) })}`),
  async logout() {
    if (!csrfToken) await api.me()
    await request<null>('/api/v1/auth/logout', {})
    csrfToken = ''
  },
}
