import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { components } from '@/api/schema'
import { money } from './overview-data'
import { orderStatuses, type OrderFilters } from './orders-data'

export const orderInput = 'min-h-11 w-full min-w-0 rounded-control border border-border-strong bg-surface px-3 py-2 text-label text-text'
const buttonBase = 'inline-flex min-h-11 items-center justify-center gap-2 rounded-control border px-4 py-2 text-label transition-colors disabled:cursor-not-allowed disabled:opacity-50'
export const orderButton = `${buttonBase} border-border-strong bg-surface hover:bg-hover`
export const orderPrimary = `${buttonBase} border-transparent bg-primary text-primary-fg hover:opacity-90`

export function OrderStatus({ status }: { status: string }) {
  const { t } = useTranslation()
  const colors: Record<string, string> = { paid: 'bg-dot-ok', pending: 'bg-dot-wait', expired: 'bg-dot-off', refunded: 'bg-dot-info' }
  const known = orderStatuses.find((value) => value === status)
  return <span className="inline-flex items-center gap-2 whitespace-nowrap text-label"><span aria-hidden="true" className={`size-[7px] rounded-full ${colors[status] ?? 'bg-dot-off'}`} />{t(known ? `merchant.overview.statuses.${known}` : 'merchant.overview.unknownStatus')}</span>
}

export function OrderChannel({ channel }: { channel: string | null }) {
  const { t } = useTranslation()
  return t(channel === 'alipay' || channel === 'wxpay' || channel === 'qqpay' ? `merchant.overview.channels.${channel}` : 'merchant.overview.channels.unassigned')
}

export function OrderTime({ value }: { value: string | null }) {
  const { i18n } = useTranslation()
  if (!value || Number.isNaN(Date.parse(value))) return <>—</>
  return <time dateTime={value}>{new Intl.DateTimeFormat(i18n.resolvedLanguage, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))}</time>
}

export function OrderRows({ orders, filters }: { orders: components['schemas']['PortalOrderDTO'][]; filters: OrderFilters }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  const orderLink = (order: components['schemas']['PortalOrderDTO']) => <Link to="/orders/$tradeNo" params={{ tradeNo: order.trade_no }} search={filters} className="break-all font-mono text-[13px] text-accent-text underline-offset-4 hover:underline">{order.trade_no}</Link>
  return <>
    <div className="hidden md:block"><table className="w-full border-collapse text-label">
      <caption className="sr-only">{t('merchant.orders.title')}</caption>
      <thead><tr className="text-left">{(['tradeNo', 'product', 'channel', 'amount', 'status', 'createdAt'] as const).map((key) => <th key={key} className={`border-b-[0.5px] border-border-strong px-3 py-3 text-[13px] font-normal text-text-2 first:pl-0 last:pr-0 ${key === 'amount' ? 'text-right' : ''}`}>{t(`merchant.overview.${key}`)}</th>)}</tr></thead>
      <tbody>{orders.map((order) => <tr key={order.trade_no} className="align-top hover:bg-hover/40">
        <td className="max-w-[220px] border-b-[0.5px] border-border py-4 pr-3">{orderLink(order)}<div className="mt-1 break-all font-mono text-caption text-text-3">{order.out_trade_no}</div></td>
        <td className="max-w-[200px] break-words border-b-[0.5px] border-border px-3 py-4">{order.name}</td>
        <td className="border-b-[0.5px] border-border px-3 py-4 text-text-2"><OrderChannel channel={order.channel} /></td>
        <td className="whitespace-nowrap border-b-[0.5px] border-border px-3 py-4 text-right tabular-nums">{money(order.actual_cents ?? order.original_cents, language)}</td>
        <td className="border-b-[0.5px] border-border px-3 py-4"><OrderStatus status={order.status} /></td>
        <td className="border-b-[0.5px] border-border py-4 pl-3 text-caption text-text-2"><OrderTime value={order.created_at} /></td>
      </tr>)}</tbody>
    </table></div>
    <ul aria-label={t('merchant.orders.cards')} className="space-y-3 md:hidden">{orders.map((order) => <li key={order.trade_no} className="rounded-card border-[0.5px] border-border bg-surface p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2"><OrderStatus status={order.status} /><span className="font-serif text-xl tabular-nums">{money(order.actual_cents ?? order.original_cents, language)}</span></div>
      <p className="mb-1 break-words text-label">{order.name}</p>{orderLink(order)}
      <p className="mt-1 break-all font-mono text-caption text-text-3">{t('merchant.orders.outTradeNo')} · {order.out_trade_no}</p>
      <div className="mt-3 flex flex-wrap justify-between gap-2 text-caption text-text-2"><OrderChannel channel={order.channel} /><OrderTime value={order.created_at} /></div>
    </li>)}</ul>
  </>
}
