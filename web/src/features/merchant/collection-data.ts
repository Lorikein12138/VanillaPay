import type { QueryClient } from '@tanstack/react-query'
import { APIError } from '@/api/client'
import type { components, operations } from '@/api/schema'

export type Schema = components['schemas']
export type ReceiptFilters = Pick<NonNullable<operations['get_api_v1_merchant_receipts']['parameters']['query']>, 'start' | 'end' | 'channel' | 'match_state'>
export function receiptSearch(search: Record<string, unknown>): ReceiptFilters {
  const result: ReceiptFilters = {}
  for (const key of ['start', 'end', 'channel', 'match_state'] as const) {
    const value = search[key]
    if (typeof value === 'string' && value.trim()) result[key] = value.trim()
  }
  return result
}

// Parse the decimal string directly: avoid floating point rounding of yuan.
export function yuanToCents(value: string): number | null {
  if (!/^\d+(\.\d{1,2})?$/.test(value)) return null
  const [yuan = '', fraction = ''] = value.split('.')
  const cents = Number(yuan) * 100 + Number(fraction.padEnd(2, '0'))
  return Number.isSafeInteger(cents) ? cents : null
}
export function centsToYuan(value: number) { return (value / 100).toFixed(2) }
export function collectionInput(ttl: string, amount: string, direction: string, bounds: Schema['SettingsCollectionSettings']['bounds']): Schema['SettingsSaveCollectionRequest'] | null {
  const seconds = ttl === '' ? null : Number(ttl)
  const cents = amount === '' ? null : yuanToCents(amount)
  if (seconds !== null && (!/^\d+$/.test(ttl) || !Number.isSafeInteger(seconds) || seconds < 60 || seconds > Math.min(86400, bounds.ttl_max_seconds))) return null
  if (amount !== '' && (cents === null || cents < Math.max(0, bounds.float_min_cents) || cents > Math.min(999, bounds.float_max_cents))) return null
  if (direction !== '' && direction !== 'up' && direction !== 'down') return null
  return { order_ttl_seconds: seconds, float_limit_cents: cents, float_direction: direction || null }
}
export function uploadError(file: File) {
  if (!['image/png', 'image/jpeg'].includes(file.type)) return 'polish.onlyPngAndJpegImagesAreSupported'
  if (!file.size || file.size > 5 * 1024 * 1024) return 'polish.theImageMustBeNonemptyAndNo'
  return null
}
export function moduleError(error: unknown): string {
  if (!(error instanceof APIError)) return error instanceof Error ? error.message : 'polish.theOperationFailedPleaseTryAgain'
  const messages: Partial<Record<Schema['ErrorCode'], string>> = {
    CSRF_FAILED: 'polish.yourSecurityTokenExpiredRefreshThePage', INVALID_CODE: 'polish.theEmailCodeIsInvalidOrExpired', INVALID_2FA: 'polish.theAuthenticatorCodeIsInvalidOrAlready',
    INVALID_QR: 'polish.theChannelSCodeCouldNotBe', UPLOAD_TOO_LARGE: 'polish.theImageMustNotExceed5Mib', UNSUPPORTED_MEDIA_TYPE: 'polish.onlyPngAndJpegImagesAreSupported',
    TTL_OUT_OF_RANGE: 'polish.theOrderLifetimeExceedsPlatformLimitsReload', FLOAT_OUT_OF_RANGE: 'polish.theAdjustmentAmountExceedsPlatformLimitsReload', CHANNEL_NOT_ALLOWED: 'polish.thisChannelIsUnavailable', CHANNEL_LOCKED: 'polish.thisChannelIsLockedTryAgainLater',
    VALIDATION_ERROR: 'polish.checkTheInputFormatAndRangeThen',
    QR_UNAVAILABLE: 'polish.uploadACollectionCodeForThisChannel', DEVICE_ALREADY_BOUND: 'polish.aDeviceIsAlreadyBoundRefreshThe',
    INVALID_PUBLIC_KEY: 'polish.invalidPublicKeyUseASupportedRsa', MAIL_UNAVAILABLE: 'polish.verificationEmailIsTemporarilyUnavailableTryAgain',
  }
  if (error.problem && messages[error.problem.code]) return messages[error.problem.code]!
  if (error.status === 0) return 'polish.theNetworkConnectionFailedPleaseTryAgain'
  if (error.status === 409) return 'polish.theStateOrVersionChangedAndHas'
  if (error.status === 429) return 'polish.tooManyRequestsWaitForTheCountdown'
  if (error.status === 404) return 'polish.theRecordDoesNotExistOrHas'
  if (error.status === 401) return 'polish.yourSessionExpiredPleaseSignInAgain'
  if (error.status === 403) return 'polish.youDoNotHavePermissionToPerform'
  return 'polish.theServiceIsTemporarilyUnavailableTryAgain'
}
export async function invalidateCollection(client: QueryClient, name: string) {
  await Promise.all(['collection', 'codes', 'device', 'receipts', 'orders', 'order', 'integration', 'stats'].map((part) => client.invalidateQueries({ queryKey: ['merchant', name, part] })))
}
export const unmatchedReasons: Record<string, 'polish.noMatchingOrderFound' | 'polish.merchantSuspended' | 'polish.orderExpired'> = { no_matching_order: 'polish.noMatchingOrderFound', merchant_banned: 'polish.merchantSuspended', expired: 'polish.orderExpired' }

export function validWhitelist(entries: string[], enabled: boolean) {
  if (entries.length > 100 || (enabled && !entries.length)) return false
  return entries.every((entry) => {
    const [address = '', prefix, extra] = entry.split('/')
    if (extra !== undefined) return false
    const ipv4 = /^(\d{1,3}\.){3}\d{1,3}$/.test(address) && address.split('.').every((part) => Number(part) <= 255)
    // A basic IPv6 shape check; the server performs complete netip validation.
    const ipv6 = address.includes(':') && /^[\da-f:.]+$/i.test(address)
    return (ipv4 || ipv6) && (prefix === undefined || (/^\d+$/.test(prefix) && Number(prefix) <= (ipv4 ? 32 : 128)))
  })
}
