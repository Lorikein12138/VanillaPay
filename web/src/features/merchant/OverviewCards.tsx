import { useTranslation } from 'react-i18next'
import type { components } from '@/api/schema'
import { money, relativeTime } from './overview-data'

type Schema = components['schemas']

export function Skeleton({ className }: { className: string }) {
  return <div aria-hidden="true" className={`animate-pulse rounded-control bg-muted motion-reduce:animate-none ${className}`} />
}

export function QueryProblem({ retry }: { retry: () => void }) {
  const { t } = useTranslation()
  return <div role="alert" className="flex flex-wrap items-center gap-x-3 gap-y-1 text-label text-text-2"><span>{t('merchant.overview.loadError')}</span><button type="button" onClick={retry} className="min-h-11 px-1 text-accent-text underline underline-offset-4 hover:text-accent-text-hover">{t('error.retry')}</button></div>
}

export function TrendChart({ points, format }: { points: Schema['PortalTrendPoint'][]; format: (cents: number) => string }) {
  const { t } = useTranslation()
  const max = Math.max(0, ...points.map((point) => point.amount_cents))
  return <div className="relative">
    {max === 0 && <p className="absolute inset-x-0 top-5 text-center text-label text-text-3">{t('merchant.overview.noReceipts')}</p>}
    <div role="img" aria-label={t('merchant.overview.trend')} className="flex h-[180px] items-end gap-[clamp(8px,2vw,28px)] border-b-[0.5px] border-border-strong">
      {points.map((point, index) => <div key={point.date} aria-hidden="true" className="flex min-w-0 flex-1 flex-col items-center gap-2"><span className="font-mono text-[11px] text-text-3">{format(point.amount_cents)}</span><div data-date={point.date} className={`w-full max-w-10 rounded-t-[4px] ${index === points.length - 1 ? 'bg-accent' : 'bg-oat'}`} style={{ height: max === 0 ? 0 : 130 * point.amount_cents / max }} /></div>)}
    </div>
    <div aria-hidden="true" className="mt-2 flex gap-[clamp(8px,2vw,28px)]">{points.map((point, index) => <span key={point.date} className="min-w-0 flex-1 text-center text-caption text-text-2">{index === points.length - 1 ? t('merchant.overview.range.1') : point.date.slice(5).replace('-', '/')}</span>)}</div>
    <table className="sr-only"><caption>{t('merchant.overview.trend')} · {t('merchant.overview.thousands')}</caption><thead><tr><th>{t('merchant.overview.date')}</th><th>{t('merchant.overview.amount')}</th></tr></thead><tbody>{points.map((point) => <tr key={point.date}><td>{point.date}</td><td>{format(point.amount_cents)}</td></tr>)}</tbody></table>
  </div>
}

export function DeviceCard({ device, now }: { device: Schema['DeviceDeviceDTO']; now: Date }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  if (!device.bound) return <p className="border-t-[0.5px] border-border py-4 text-label text-text-2">{t('merchant.overview.unbound')}</p>
  const last = device.online ? device.last_seen_at : device.offline_at ?? device.last_seen_at
  return <div className="flex flex-wrap items-center justify-between gap-3 border-t-[0.5px] border-border py-3.5">
    <div className="min-w-0"><p className="break-words text-[15px]">{device.name ?? t('merchant.overview.unnamed')}</p><p className="mt-1 text-caption text-text-3">{last ? t(device.online ? 'merchant.overview.heartbeat' : 'merchant.overview.lastOnline', { time: relativeTime(last, now, language) }) : t('merchant.overview.neverSeen')}</p></div>
    <span className={`inline-flex items-center gap-2 whitespace-nowrap text-label ${device.online ? '' : 'text-text-2'}`}><span aria-hidden="true" className={`size-[7px] rounded-full ${device.online ? 'bg-dot-ok' : 'bg-dot-off'}`} />{t(device.online ? 'merchant.overview.online' : 'merchant.overview.offline')}</span>
  </div>
}

const tones: Record<string, string> = { paid: 'bg-dot-ok', pending: 'bg-dot-wait', expired: 'bg-dot-off', refunded: 'bg-dot-info' }
export function RecentOrders({ orders }: { orders: Schema['PortalOrderDTO'][] }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  const date = new Intl.DateTimeFormat(language, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' })
  if (orders.length === 0) return <p className="min-h-80 border-t-[0.5px] border-border py-10 text-center text-label text-text-3">{t('merchant.overview.noOrders')}</p>
  return <div role="region" aria-label={t('merchant.overview.recentOrders')} tabIndex={0} className="overflow-x-auto"><table className="w-full min-w-[760px] border-collapse text-label">
    <thead><tr className="text-left">{(['tradeNo', 'product', 'channel', 'amount', 'status', 'createdAt'] as const).map((key) => <th key={key} className={`border-b-[0.5px] border-border-strong px-4 py-2.5 text-[13px] font-normal text-text-2 first:pl-0 last:pr-0 ${key === 'amount' || key === 'createdAt' ? 'text-right' : ''}`}>{t(`merchant.overview.${key}`)}</th>)}</tr></thead>
    <tbody>{orders.map((order) => <tr key={order.trade_no}>
      <td className="border-b-[0.5px] border-border py-4 pr-4 font-mono text-[13px]">{order.trade_no}</td>
      <td className="border-b-[0.5px] border-border p-4">{order.name}</td>
      <td className="border-b-[0.5px] border-border p-4 text-text-2">{t(order.channel === 'alipay' || order.channel === 'wxpay' || order.channel === 'qqpay' ? `merchant.overview.channels.${order.channel}` : 'merchant.overview.channels.unassigned')}</td>
      <td className="border-b-[0.5px] border-border p-4 text-right tabular-nums">{money(order.actual_cents ?? order.original_cents, language)}</td>
      <td className="border-b-[0.5px] border-border p-4"><span className="inline-flex items-center gap-2 whitespace-nowrap"><span aria-hidden="true" className={`size-[7px] rounded-full ${tones[order.status] ?? 'bg-dot-off'}`} />{t(order.status === 'paid' || order.status === 'pending' || order.status === 'expired' || order.status === 'refunded' ? `merchant.overview.statuses.${order.status}` : 'merchant.overview.unknownStatus')}</span></td>
      <td className="whitespace-nowrap border-b-[0.5px] border-border py-4 pl-4 text-right text-text-2"><time dateTime={order.created_at}>{date.format(new Date(order.created_at))}</time></td>
    </tr>)}</tbody>
  </table></div>
}
