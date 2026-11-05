import type { components, operations } from '@/api/schema'

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

function rejectResponse(response: Response, problem?: Schema['ErrorEnvelope']['error'], protectedRequest = true): never {
  if (response.status === 401 && protectedRequest && problem?.code !== 'INVALID_2FA') {
    csrfToken = ''
    unauthorized?.()
  }
  throw new APIError(response.status, problem)
}

async function request<T>(path: string, body?: unknown, protectedRequest = true, idempotencyKey?: string, method?: string, signal?: AbortSignal): Promise<T> {
  let response: Response
  try {
    response = await fetch(path, {
      method: method ?? (body === undefined ? 'GET' : 'POST'),
      signal,
      credentials: 'include',
      headers: {
        Accept: 'application/json',
        ...(body === undefined || body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
        ...(body !== undefined && protectedRequest && csrfToken ? { 'X-CSRF-Token': csrfToken } : {}),
        ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
      },
      ...(body === undefined ? {} : { body: body instanceof FormData ? body : JSON.stringify(body) }),
    })
  } catch {
    throw new APIError(0)
  }
  const envelope = await response.json().catch(() => null) as Schema['ErrorEnvelope'] | (Schema['SuccessEnvelope'] & { data: T }) | null
  if (!response.ok) {
    rejectResponse(response, envelope && 'error' in envelope && envelope.error ? envelope.error : undefined, protectedRequest)
  }
  if (!envelope || !('data' in envelope)) throw new APIError(502)
  return envelope.data
}

type OrdersQuery = NonNullable<operations['get_api_v1_merchant_orders']['parameters']['query']>
function queryString(value: Record<string, string | number | undefined>) {
  return new URLSearchParams(Object.entries(value).filter(([, item]) => item !== undefined && item !== '').map(([key, item]) => [key, String(item)]))
}

async function exportOrders(filters: OrdersQuery) {
  let response: Response
  try { response = await fetch(`/api/v1/merchant/orders/export.csv?${queryString(filters)}`, { credentials: 'include', headers: { Accept: 'text/csv' } }) }
  catch { throw new APIError(0) }
  if (!response.ok) {
    const envelope = await response.json().catch(() => null) as Schema['ErrorEnvelope'] | null
    rejectResponse(response, envelope?.error)
  }
  let filename = `orders-${new Date().toISOString().slice(0, 10)}.csv`
  const supplied = /filename\*?=(?:UTF-8''|"?)([^";]+)/i.exec(response.headers.get('Content-Disposition') ?? '')?.[1]
  if (supplied) {
    try {
      const decoded = decodeURIComponent(supplied)
      if (!/[\\/\r\n]/.test(decoded) && decoded.endsWith('.csv')) filename = decoded
    } catch { /* Use the local filename when the header is malformed. */ }
  }
  try { return { blob: await response.blob(), filename } }
  catch { throw new APIError(0) }
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
  collection: () => request<Schema['Collection']>('/api/v1/merchant/collection'),
  qrSlots: () => request<Schema['QRSlots']>('/api/v1/merchant/collection/codes'),
  uploadQR: (channel: string, file: File) => {
    const form = new FormData(); form.set('file', file)
    return request<Schema['SettingsQRDTO']>(`/api/v1/merchant/collection/codes/${encodeURIComponent(channel)}`, form, true, undefined, 'PUT')
  },
  setQREnabled: (channel: string, enabled: boolean) => request<Schema['SettingsQRDTO']>(`/api/v1/merchant/collection/codes/${encodeURIComponent(channel)}`, { enabled }, true, undefined, 'PATCH'),
  saveCollection: (value: Schema['SettingsSaveCollectionRequest']) => request<Schema['SettingsCollectionSettings']>('/api/v1/merchant/collection/settings', value, true, undefined, 'PUT'),
  createBinding: (replace_confirmed: boolean) => request<Schema['DeviceBindingDTO']>('/api/v1/merchant/device/bindings', { replace_confirmed }),
  binding: (id: string, signal?: AbortSignal) => request<Schema['DeviceBindingStatus']>(`/api/v1/merchant/device/bindings/${encodeURIComponent(id)}`, undefined, true, undefined, undefined, signal),
  unbindDevice: () => request<null>('/api/v1/merchant/device', { confirm: true }, true, undefined, 'DELETE'),
  receipts: (filters: NonNullable<operations['get_api_v1_merchant_receipts']['parameters']['query']>) => request<Schema['ReceiptsPage']>(`/api/v1/merchant/receipts?${queryString(filters)}`),
  receipt: (id: string) => request<Schema['PortalReceiptDTO']>(`/api/v1/merchant/receipts/${encodeURIComponent(id)}`),
  integration: () => request<Schema['Integration']>('/api/v1/merchant/integration'),
  emailCode: (value: Schema['AuthEmailCodeRequest']) => request<Schema['AuthEmailCodeResult']>('/api/v1/auth/email-codes', value),
  resetKey: (security_proof: Schema['AuthSecurityProof']) => request<Schema['MerchantKeyDTO']>('/api/v1/merchant/integration/reset-key', { security_proof }),
  saveRSA: (value: Schema['MerchantRSAKeyRequest']) => request<operations['put_api_v1_merchant_integration_rsa_public_key']['responses'][200]['content']['application/json']['data']>('/api/v1/merchant/integration/rsa-public-key', value, true, undefined, 'PUT'),
  saveWhitelist: (value: Schema['MerchantIPWhitelist']) => request<Schema['MerchantIPWhitelist']>('/api/v1/merchant/integration/ip-whitelist', value, true, undefined, 'PUT'),
  orders: (limit: number) => request<Schema['OrdersPage']>(`/api/v1/merchant/orders?${new URLSearchParams({ limit: String(limit) })}`),
  orderPage: (filters: OrdersQuery) => request<Schema['OrdersPage']>(`/api/v1/merchant/orders?${queryString(filters)}`),
  order: (tradeNo: string) => request<Schema['OrderDetail']>(`/api/v1/merchant/orders/${encodeURIComponent(tradeNo)}`),
  supplement: (tradeNo: string, value: Schema['PaymentSupplementRequest'], key: string) => request<Schema['PortalOrderDTO']>(`/api/v1/merchant/orders/${encodeURIComponent(tradeNo)}/supplement`, value, true, key),
  refund: (tradeNo: string, value: operations['post_api_v1_merchant_orders_trade_no_refund']['requestBody']['content']['application/json'], key: string) => request<Schema['PortalOrderDTO']>(`/api/v1/merchant/orders/${encodeURIComponent(tradeNo)}/refund`, value, true, key),
  resolveRefund: (id: string, value: Schema['PaymentRefundResolution'], key: string) => request<Schema['PaymentRefundDTO']>(`/api/v1/merchant/refunds/${encodeURIComponent(id)}/resolve`, value, true, key),
  replayCallback: (tradeNo: string, key: string) => request<operations['post_api_v1_merchant_orders_trade_no_callback_replay']['responses'][202]['content']['application/json']['data']>(`/api/v1/merchant/orders/${encodeURIComponent(tradeNo)}/callback-replay`, {}, true, key),
  exportOrders,
  async logout() {
    if (!csrfToken) await api.me()
    await request<null>('/api/v1/auth/logout', {})
    csrfToken = ''
  },
}
