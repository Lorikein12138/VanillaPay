import type { components, operations } from '@/api/schema'
import { notifySessionCookieChange, sessionCookie } from './session-cookie'

type Schema = components['schemas']
export class APIError extends Error {
  constructor(readonly status: number, readonly problem?: Schema['ErrorEnvelope']['error']) {
    super(problem?.code ?? 'REQUEST_FAILED')
  }
}

export type Role = 'merchant' | 'console'
const csrfTokens: Record<Role, string> = { merchant: '', console: '' }
const sessionVersions: Record<Role, number> = { merchant: 0, console: 0 }
const verifiedCookies: Partial<Record<Role, string>> = {}
let unauthorized: ((role: Role) => void) | undefined

export function onUnauthorized(handler: (role: Role) => void) {
  unauthorized = handler
}

function remember<T extends { csrf_token: string }>(session: T, role: Role = 'merchant'): T {
  verifiedCookies[role] = sessionCookie(role)
  csrfTokens[role] = session.csrf_token
  return session
}

function forgetSession(role: Role) {
  sessionVersions[role]++
  csrfTokens[role] = ''
  delete verifiedCookies[role]
}

function startSession<T extends { csrf_token: string }>(session: T, role: Role = 'merchant'): T {
  sessionVersions[role]++
  notifySessionCookieChange()
  return remember(session, role)
}

function rejectResponse(response: Response, problem: Schema['ErrorEnvelope']['error'] | undefined, protectedRequest: boolean, role: Role, version: number): never {
  if (response.status === 401 && protectedRequest && problem?.code !== 'INVALID_2FA' && problem?.code !== 'INVALID_CREDENTIALS' && problem?.code !== 'CHALLENGE_EXPIRED') {
    if (sessionVersions[role] === version && (verifiedCookies[role] === undefined || verifiedCookies[role] === sessionCookie(role))) {
      forgetSession(role)
      unauthorized?.(role)
    }
  }
  throw new APIError(response.status, problem)
}

async function request<T>(path: string, body?: unknown, protectedRequest = true, idempotencyKey?: string, method?: string, signal?: AbortSignal, csrfOverride?: string, noContent?: T): Promise<T> {
  const role = path.startsWith('/api/v1/console/') ? 'console' : 'merchant'
  const version = sessionVersions[role]
  const csrf = csrfOverride ?? (protectedRequest ? csrfTokens[role] : '')
  let response: Response
  try {
    response = await fetch(path, {
      method: method ?? (body === undefined ? 'GET' : 'POST'),
      signal,
      credentials: 'include',
      headers: {
        Accept: 'application/json',
        ...(body === undefined || body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
        ...(body !== undefined && csrf ? { 'X-CSRF-Token': csrf } : {}),
        ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
      },
      ...(body === undefined ? {} : { body: body instanceof FormData ? body : JSON.stringify(body) }),
    })
  } catch {
    throw new APIError(0)
  }
  const envelope = await response.json().catch(() => null) as Schema['ErrorEnvelope'] | (Schema['SuccessEnvelope'] & { data: T }) | null
  if (!response.ok) {
    rejectResponse(response, envelope && 'error' in envelope && envelope.error ? envelope.error : undefined, protectedRequest && !signal?.aborted, role, version)
  }
  if (response.status === 204 && noContent !== undefined) return noContent
  if (!envelope || !('data' in envelope)) throw new APIError(502)
  return envelope.data
}

async function readSession(role: Role, protectedRequest: boolean, signal?: AbortSignal) {
  const version = sessionVersions[role]
  const cookie = sessionCookie(role)
  const session = await request<Schema['SessionIdentity']>(`/api/v1/${role}/me`, undefined, protectedRequest, undefined, 'GET', signal)
  if (signal?.aborted || sessionVersions[role] !== version || sessionCookie(role) !== cookie ||
    (cookie && session.csrf_token !== cookie)) throw new APIError(0)
  if (csrfTokens[role] !== session.csrf_token || verifiedCookies[role] !== cookie) sessionVersions[role]++
  return remember(session, role)
}

type OrdersQuery = NonNullable<operations['get_api_v1_merchant_orders']['parameters']['query']>
type AuditQuery = NonNullable<operations['get_api_v1_console_audit_logs']['parameters']['query']>
// Keep the int64 ID as decimal text through URL serialization.
export type ConsoleAuditQuery = Omit<AuditQuery, 'merchant_id'> & { merchant_id?: string }
function queryString(value: Record<string, string | number | boolean | undefined>) {
  return new URLSearchParams(Object.entries(value).filter(([, item]) => item !== undefined && item !== '').map(([key, item]) => [key, String(item)]))
}

async function exportOrders(filters: OrdersQuery, role: Role = 'merchant') {
  const version = sessionVersions[role]
  let response: Response
  try { response = await fetch(`/api/v1/${role}/orders/export.csv?${queryString(filters)}`, { credentials: 'include', headers: { Accept: 'text/csv' } }) }
  catch { throw new APIError(0) }
  if (!response.ok) {
    const envelope = await response.json().catch(() => null) as Schema['ErrorEnvelope'] | null
    rejectResponse(response, envelope?.error, true, role, version)
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
  checkout: (tradeNo: string, signal?: AbortSignal) => request<Schema['PaymentCheckoutDTO']>(`/api/v1/checkout/${encodeURIComponent(tradeNo)}`, undefined, false, undefined, 'GET', signal),
  verifyCheckout: (tradeNo: string, value: operations['post_api_v1_checkout_tradeNo_verify']['requestBody']['content']['application/json']) => request<null>(`/api/v1/checkout/${encodeURIComponent(tradeNo)}/verify`, value, false, undefined, 'POST', undefined, undefined, null),
  selectCheckoutChannel: (tradeNo: string, value: operations['post_api_v1_checkout_tradeNo_channel']['requestBody']['content']['application/json'], signal?: AbortSignal) => request<Schema['PaymentCheckoutDTO']>(`/api/v1/checkout/${encodeURIComponent(tradeNo)}/channel`, value, false, undefined, 'POST', signal),
  subscription: () => request<Schema['SubscriptionOverview']>('/api/v1/merchant/subscription'),
  purchase: (value: Schema['SubscriptionPurchaseRequest'], key: string) => request<Schema['SubscriptionPurchaseDTO']>('/api/v1/merchant/subscription/purchases', value, true, key),
  purchaseDetail: (id: string) => request<Schema['SubscriptionPurchaseDTO']>(`/api/v1/merchant/subscription/purchases/${encodeURIComponent(id)}`),
  cancelPurchase: (id: string) => request<Schema['SubscriptionPurchaseDTO']>(`/api/v1/merchant/subscription/purchases/${encodeURIComponent(id)}/cancel`, {}),
  notifications: (filters: NonNullable<operations['get_api_v1_merchant_notifications']['parameters']['query']>) => request<Schema['PortalNotificationPage']>(`/api/v1/merchant/notifications?${queryString({ ...filters, unread_only: filters.unread_only === undefined ? undefined : String(filters.unread_only) })}`),
  readNotifications: (value: Schema['PortalReadRequest']) => request<Schema['PortalReadResult']>('/api/v1/merchant/notifications/read', value),
  readAnnouncement: (id: string, value: operations['post_api_v1_merchant_announcements_id_read']['requestBody']['content']['application/json']) => request<null>(`/api/v1/merchant/announcements/${encodeURIComponent(id)}/read`, value),
  auditLogs: (filters: Pick<NonNullable<operations['get_api_v1_merchant_audit_logs']['parameters']['query']>, 'start' | 'end' | 'action' | 'result' | 'cursor' | 'limit'>) => request<Schema['AuditsPage']>(`/api/v1/merchant/audit-logs?${queryString(filters)}`),
  config: (signal?: AbortSignal) => request<Schema['PublicConfig']>('/api/v1/public/config', undefined, false, undefined, 'GET', signal),
  legal: (kind: Schema['LegalVersion']['kind']) => request<Schema['LegalVersion']>(`/api/v1/public/legal/${kind}`, undefined, false),
  register: (value: Schema['AuthRegisterRequest']) => request<Schema['RegisteredUser']>('/api/v1/auth/register', value, false),
  resetPassword: (value: Schema['AuthResetPasswordRequest']) => request<null>('/api/v1/auth/reset-password', value, false),
  forgetSession: () => { forgetSession('merchant'); notifySessionCookieChange() },
  changeUsername: (value: operations['patch_api_v1_merchant_profile_username']['requestBody']['content']['application/json']) => request<Schema['AuthUserDTO']>('/api/v1/merchant/profile/username', value, true, undefined, 'PATCH'),
  changeEmail: (value: operations['patch_api_v1_merchant_profile_email']['requestBody']['content']['application/json']) => request<Schema['AuthUserDTO']>('/api/v1/merchant/profile/email', value, true, undefined, 'PATCH'),
  changePassword: (value: operations['patch_api_v1_merchant_profile_password']['requestBody']['content']['application/json']) => request<null>('/api/v1/merchant/profile/password', value, true, undefined, 'PATCH'),
  setupTOTP: (value: operations['post_api_v1_merchant_2fa_setup']['requestBody']['content']['application/json']) => request<Schema['AuthTOTPSetup']>('/api/v1/merchant/2fa/setup', value),
  confirmTOTP: (value: Schema['AuthTOTPConfirm']) => request<Schema['TOTPConfirmed']>('/api/v1/merchant/2fa/confirm', value),
  disableTOTP: (value: Schema['AuthFactorProof']) => request<Schema['TOTPDisabled']>('/api/v1/merchant/2fa', value, true, undefined, 'DELETE'),
  preferences: (value: Schema['Preferences']) => request<Schema['Preferences']>('/api/v1/merchant/preferences', value, true, undefined, 'PATCH'),
  async login(value: Schema['AuthLoginRequest']) {
    const result = await request<Schema['AuthSessionResult'] | Schema['AuthLoginResult']>('/api/v1/auth/login', value, false)
    return 'requires_2fa' in result ? result : startSession(result)
  },
  async secondFactor(value: Schema['AuthFactorRequest']) {
    return startSession(await request<Schema['AuthSessionResult']>('/api/v1/auth/second-factor', value, false))
  },
  peekSession: (signal: AbortSignal) => request<Schema['SessionIdentity']>('/api/v1/merchant/me', undefined, false, undefined, 'GET', signal),
  me: (protectedRequest = true, signal?: AbortSignal) => readSession('merchant', protectedRequest, signal),
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
  emailCode: (value: Schema['AuthEmailCodeRequest']) => request<Schema['AuthEmailCodeResult']>('/api/v1/auth/email-codes', value, value.purpose !== 'register' && value.purpose !== 'reset_password'),
  resetKey: (security_proof: Schema['AuthSecurityProof']) => request<Schema['MerchantKeyDTO']>('/api/v1/merchant/integration/reset-key', { security_proof }),
  saveRSA: (value: Schema['MerchantRSAKeyRequest']) => request<operations['put_api_v1_merchant_integration_rsa_public_key']['responses'][200]['content']['application/json']['data']>('/api/v1/merchant/integration/rsa-public-key', value, true, undefined, 'PUT'),
  saveWhitelist: (value: Schema['MerchantIPWhitelist']) => request<Schema['MerchantIPWhitelist']>('/api/v1/merchant/integration/ip-whitelist', value, true, undefined, 'PUT'),
  notifyDomains: (signal?: AbortSignal) => request<Schema['MerchantNotifyDomains']>('/api/v1/merchant/integration/notify-domains', undefined, true, undefined, 'GET', signal),
  saveNotifyDomains: (value: Schema['MerchantNotifyDomains']) => request<Schema['MerchantNotifyDomains']>('/api/v1/merchant/integration/notify-domains', value, true, undefined, 'PUT'),
  callbackSettings: (signal?: AbortSignal) => request<Schema['MerchantCallbackSettings']>('/api/v1/merchant/integration/callback-settings', undefined, true, undefined, 'GET', signal),
  saveCallbackSettings: (value: Schema['MerchantCallbackSettings']) => request<Schema['MerchantCallbackSettings']>('/api/v1/merchant/integration/callback-settings', value, true, undefined, 'PUT'),
  testPayment: (value: Schema['MerchantTestPaymentRequest']) => request<Schema['MerchantTestPayment']>('/api/v1/merchant/test-payment', value),
  orders: (limit: number) => request<Schema['OrdersPage']>(`/api/v1/merchant/orders?${new URLSearchParams({ limit: String(limit) })}`),
  orderPage: (filters: OrdersQuery) => request<Schema['OrdersPage']>(`/api/v1/merchant/orders?${queryString(filters)}`),
  order: (tradeNo: string) => request<Schema['OrderDetail']>(`/api/v1/merchant/orders/${encodeURIComponent(tradeNo)}`),
  supplement: (tradeNo: string, value: Schema['PaymentSupplementRequest'], key: string) => request<Schema['PortalOrderDTO']>(`/api/v1/merchant/orders/${encodeURIComponent(tradeNo)}/supplement`, value, true, key),
  refund: (tradeNo: string, value: operations['post_api_v1_merchant_orders_trade_no_refund']['requestBody']['content']['application/json'], key: string) => request<Schema['PortalOrderDTO']>(`/api/v1/merchant/orders/${encodeURIComponent(tradeNo)}/refund`, value, true, key),
  resolveRefund: (id: string, value: Schema['PaymentRefundResolution'], key: string) => request<Schema['PaymentRefundDTO']>(`/api/v1/merchant/refunds/${encodeURIComponent(id)}/resolve`, value, true, key),
  replayCallback: (tradeNo: string, key: string) => request<operations['post_api_v1_merchant_orders_trade_no_callback_replay']['responses'][202]['content']['application/json']['data']>(`/api/v1/merchant/orders/${encodeURIComponent(tradeNo)}/callback-replay`, {}, true, key),
  exportOrders,
  async logout() {
    if (!csrfTokens.merchant) await api.me()
    try { await request<null>('/api/v1/auth/logout', {}) }
    finally { notifySessionCookieChange() }
    forgetSession('merchant')
  },
}

export type ConsoleRecovery = NonNullable<operations['post_api_v1_console_recovery']['responses'][200]['content']['application/json']['data']>
export const consoleApi = {
  auditLogs: (filters: ConsoleAuditQuery, signal?: AbortSignal) =>
    request<Schema['AuditsPage']>(`/api/v1/console/audit-logs?${queryString(filters)}`, undefined, true, undefined, 'GET', signal),
  announcements: (filters: NonNullable<operations['get_api_v1_console_announcements']['parameters']['query']>, signal?: AbortSignal) =>
    request<Schema['AnnouncementsPage']>(`/api/v1/console/announcements?${queryString(filters)}`, undefined, true, undefined, 'GET', signal),
  publishAnnouncement: (value: Pick<Schema['PortalAnnouncementCommand'], 'title' | 'body' | 'popup'>) =>
    request<Schema['PortalAnnouncementDTO']>('/api/v1/console/announcements', value),
  updateAnnouncement: (id: string, value: Pick<Schema['PortalAnnouncementCommand'], 'title' | 'body' | 'popup'> & { revision: number }) =>
    request<Schema['PortalAnnouncementDTO']>(`/api/v1/console/announcements/${encodeURIComponent(id)}`, value, true, undefined, 'PUT'),
  deleteAnnouncement: (id: string, value: operations['delete_api_v1_console_announcements_id']['requestBody']['content']['application/json']) =>
    request<null>(`/api/v1/console/announcements/${encodeURIComponent(id)}`, value, true, undefined, 'DELETE'),
  merchants: (filters: NonNullable<operations['get_api_v1_console_merchants']['parameters']['query']>, signal?: AbortSignal) => request<Schema['ConsoleMerchantPage']>(`/api/v1/console/merchants?${queryString(filters)}`, undefined, true, undefined, 'GET', signal),
  merchant: (id: string, signal?: AbortSignal) => request<Schema['ConsoleMerchantDetail']>(`/api/v1/console/merchants/${encodeURIComponent(id)}`, undefined, true, undefined, 'GET', signal),
  createMerchant: (value: Schema['MerchantCreateMerchant']) => request<Schema['AuthUserDTO']>('/api/v1/console/merchants', value),
  banMerchant: (id: string, value: operations['post_api_v1_console_merchants_id_ban']['requestBody']['content']['application/json']) => request<Schema['MerchantBanResult']>(`/api/v1/console/merchants/${encodeURIComponent(id)}/ban`, value),
  unbanMerchant: (id: string) => request<Schema['AuthUserDTO']>(`/api/v1/console/merchants/${encodeURIComponent(id)}/unban`, {}),
  approveMerchant: (id: string) => request<Schema['MerchantApproval']>(`/api/v1/console/merchants/${encodeURIComponent(id)}/approve`, {}),
  rejectMerchant: (id: string, value: operations['post_api_v1_console_merchants_id_reject']['requestBody']['content']['application/json']) => request<Schema['MerchantApproval']>(`/api/v1/console/merchants/${encodeURIComponent(id)}/reject`, value),
  resetMerchantPassword: (id: string, value: operations['post_api_v1_console_merchants_id_reset_password']['requestBody']['content']['application/json']) => request<null>(`/api/v1/console/merchants/${encodeURIComponent(id)}/reset-password`, value),
  grantSubscription: (id: string, value: Schema['SubscriptionGrantRequest']) => request<Schema['SubscriptionSubscriptionDTO']>(`/api/v1/console/merchants/${encodeURIComponent(id)}/subscription`, value, true, undefined, 'PUT'),
  plans: (include_deleted = false, signal?: AbortSignal) => request<Schema['PlansPage']>(`/api/v1/console/plans?${queryString({ include_deleted })}`, undefined, true, undefined, 'GET', signal),
  savePlan: (value: Schema['SubscriptionPlanCommand']) => request<Schema['SubscriptionPlanDTO']>(value.id ? `/api/v1/console/plans/${encodeURIComponent(value.id)}` : '/api/v1/console/plans', value, true, undefined, value.id ? 'PUT' : 'POST'),
  deletePlan: (id: string, revision: number) => request<Schema['SubscriptionDeletedPlanResult']>(`/api/v1/console/plans/${encodeURIComponent(id)}`, { revision }, true, undefined, 'DELETE'),
  purchases: (filters: NonNullable<operations['get_api_v1_console_purchases']['parameters']['query']>, signal?: AbortSignal) => request<Schema['SubscriptionPurchasePage']>(`/api/v1/console/purchases?${queryString(filters)}`, undefined, true, undefined, 'GET', signal),
  settings: (signal?: AbortSignal) => request<Schema['Settings']>('/api/v1/console/settings', undefined, true, undefined, 'GET', signal),
  saveSettings: (group: operations['put_api_v1_console_settings_group']['parameters']['path']['group'], value: Schema['SettingsUpdateGroup']) => request<NonNullable<Schema['SettingsGroupResult']>>(`/api/v1/console/settings/${group}`, value, true, undefined, 'PUT'),
  testMail: (recipient: string, signal?: AbortSignal) =>
    request<NonNullable<operations['post_api_v1_console_settings_mail_test']['responses'][202]['content']['application/json']['data']>>(
      '/api/v1/console/settings/mail/test', { recipient }, true, undefined, 'POST', signal,
    ),
  mailTestStatus: (id: string, signal?: AbortSignal) => request<NonNullable<operations['get_api_v1_console_settings_mail_test_id']['responses'][200]['content']['application/json']['data']>>(`/api/v1/console/settings/mail/test/${encodeURIComponent(id)}`, undefined, true, undefined, 'GET', signal),
  uploadSiteImage(kind: 'logo' | 'favicon', file: File, signal?: AbortSignal) {
    const body = new FormData()
    body.set('file', file)
    return request<NonNullable<operations['post_api_v1_console_assets_logo']['responses'][201]['content']['application/json']['data']>>(
      `/api/v1/console/assets/${kind}`, body, true, undefined, 'POST', signal,
    )
  },
  orderPage: (filters: OrdersQuery) => request<Schema['OrdersPage']>(`/api/v1/console/orders?${queryString(filters)}`),
  order: (tradeNo: string) => request<Schema['OrderDetail']>(`/api/v1/console/orders/${encodeURIComponent(tradeNo)}`),
  exportOrders: (filters: OrdersQuery) => exportOrders(filters, 'console'),
  bootstrapStatus: () => request<NonNullable<operations['get_api_v1_console_bootstrap_status']['responses'][200]['content']['application/json']['data']>>('/api/v1/console/bootstrap-status', undefined, false),
  bootstrap: async (value: Schema['AuthBootstrapRequest']) => startSession(await request<Schema['AuthSessionResult']>('/api/v1/console/bootstrap', value, false), 'console'),
  async login(value: Schema['AuthLoginRequest']) {
    const result = await request<Schema['AuthSessionResult'] | Schema['AuthLoginResult']>('/api/v1/console/login', value, false)
    return 'requires_2fa' in result ? result : startSession(result, 'console')
  },
  secondFactor: async (value: Schema['AuthFactorRequest']) => startSession(await request<Schema['AuthSessionResult']>('/api/v1/console/second-factor', value, false), 'console'),
  me: (protectedRequest = true, signal?: AbortSignal) => readSession('console', protectedRequest, signal),
  async logout() {
    if (!csrfTokens.console) await consoleApi.me()
    try { await request<null>('/api/v1/console/logout', {}) }
    finally { notifySessionCookieChange() }
    forgetSession('console')
  },
  forgetSession: () => { forgetSession('console'); notifySessionCookieChange() },
  preferences: (value: Schema['Preferences']) => request<Schema['Preferences']>('/api/v1/console/preferences', value, true, undefined, 'PATCH'),
  setupTOTP: (value: operations['post_api_v1_console_2fa_setup']['requestBody']['content']['application/json']) => request<Schema['AuthTOTPSetup']>('/api/v1/console/2fa/setup', value),
  confirmTOTP: (value: Schema['AuthTOTPConfirm']) => request<Schema['TOTPConfirmed']>('/api/v1/console/2fa/confirm', value),
  disableTOTP: (value: Schema['AuthFactorProof']) => request<Schema['TOTPDisabled']>('/api/v1/console/2fa', value, true, undefined, 'DELETE'),
  changePassword: (value: operations['patch_api_v1_console_password']['requestBody']['content']['application/json']) => request<null>('/api/v1/console/password', value, true, undefined, 'PATCH'),
  stats: (range: { start: string; end: string; timezone: string }) => request<Schema['PortalStatsDTO']>(`/api/v1/console/stats?${new URLSearchParams(range)}`),
  recover: (recovery_code: string, signal?: AbortSignal) => request<ConsoleRecovery>('/api/v1/console/recovery', { recovery_code }, false, undefined, 'POST', signal),
  repair: (csrf: string, value: Schema['SettingsUpdateGroup'], signal?: AbortSignal) => request<Schema['SettingsGroupResult']>('/api/v1/console/settings/security', value, false, undefined, 'PUT', signal, csrf),
}
