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
  if (!['image/png', 'image/jpeg'].includes(file.type)) return '仅支持 PNG 或 JPEG 图片。'
  if (!file.size || file.size > 5 * 1024 * 1024) return '图片须大于 0 且不超过 5 MiB。'
  return null
}
export function moduleError(error: unknown): string {
  if (!(error instanceof APIError)) return error instanceof Error ? error.message : '操作失败，请重试。'
  const messages: Partial<Record<Schema['ErrorCode'], string>> = {
    CSRF_FAILED: '安全令牌已失效，请刷新页面后重试。', INVALID_CODE: '邮箱验证码无效或已过期，请重新获取。', INVALID_2FA: '动态验证码无效或已使用，请输入新的验证码。',
    INVALID_QR: '无法识别该渠道的收款码，请上传只含一个正确收款码的图片。', UPLOAD_TOO_LARGE: '图片不能超过 5 MiB。', UNSUPPORTED_MEDIA_TYPE: '仅支持 PNG 或 JPEG 图片。',
    TTL_OUT_OF_RANGE: '订单有效期超出平台允许范围，请重新加载设置。', FLOAT_OUT_OF_RANGE: '浮动金额超出平台允许范围，请重新加载设置。', CHANNEL_NOT_ALLOWED: '当前渠道不可用。', CHANNEL_LOCKED: '渠道被锁定，请稍后重试。',
    VALIDATION_ERROR: '输入格式或范围不正确，请检查后重试。',
    QR_UNAVAILABLE: '请先上传该渠道的收款码。', DEVICE_ALREADY_BOUND: '设备已被绑定，请刷新页面后选择更换设备。',
    INVALID_PUBLIC_KEY: '公钥无效，请使用后端支持的 RSA 公钥格式与强度。', MAIL_UNAVAILABLE: '验证码邮件暂时无法发送，请稍后重试。',
  }
  if (error.problem && messages[error.problem.code]) return messages[error.problem.code]!
  if (error.status === 0) return '网络连接失败，请重试。'
  if (error.status === 409) return '状态或版本已变化，已重新加载，请核对后重试。'
  if (error.status === 429) return '操作过于频繁，请等待倒计时结束后重试。'
  if (error.status === 404) return '记录不存在或已失效。'
  if (error.status === 401) return '登录已失效，请重新登录。'
  if (error.status === 403) return '没有权限执行此操作。'
  return '服务暂时不可用，请稍后重试。'
}
export async function invalidateCollection(client: QueryClient, name: string) {
  await Promise.all(['collection', 'codes', 'device', 'receipts', 'orders', 'order', 'integration', 'stats'].map((part) => client.invalidateQueries({ queryKey: ['merchant', name, part] })))
}
export const unmatchedReasons: Record<string, string> = { no_matching_order: '未找到匹配订单', merchant_banned: '商户已被封禁', expired: '订单已过期' }

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
