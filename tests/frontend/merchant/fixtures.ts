import type { components } from '@/api/schema'

type Schema = components['schemas']
export const emptyStats = (start = '', end = '', timezone = 'Asia/Shanghai'): Schema['PortalStatsDTO'] => ({ start, end, timezone, amount_basis: 'paid_at', order_basis: 'created_at', total_amount_cents: 0, total_order_count: 0, status_counts: { paid: 0, pending: 0, expired: 0 }, channels: {}, unassigned_count: 0, trend: [] })
export const unboundDevice: Schema['DeviceDeviceDTO'] = { bound: false, online: false, id: null, name: null, app_version: null, last_seen_at: null, offline_at: null, heartbeat_interval_seconds: 30, offline_threshold_seconds: 90 }
export function order(status: string, channel: string | null): Schema['PortalOrderDTO'] {
  return { trade_no: crypto.randomUUID(), out_trade_no: crypto.randomUUID(), merchant_id: '1', merchant_username: 'fixture_shop', purpose: 'external', protocol: 'v1', name: `Product ${status}`, product_id: '', param: '', channel, original_cents: 12345, actual_cents: channel ? 12346 : null, status, expire_reason: null, is_manual: false, manual_reason: null, created_at: '2026-11-05T01:02:00Z', paid_at: null, expires_at: '2026-11-06T01:02:00Z', refunded_at: null, callback_status: 'none', receipt_id: null }
}
