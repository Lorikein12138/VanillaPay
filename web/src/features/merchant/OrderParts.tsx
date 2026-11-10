import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { components } from '@/api/schema'
import { StatusDot } from '@/components/StatusDot'
import { Identifier } from '@/components/ui/layout'
import { ChannelLabel } from '@/components/ui/payment'
import { RecordLink } from '@/components/ui/records'
import { Table, Td, Th } from '@/components/ui/table'
import { money } from './overview-data'
import { orderChannels, orderStatuses, type OrderFilters } from './orders-data'

const statusTones = { paid: 'ok', pending: 'wait', expired: 'off', refunded: 'info' } as const

export function OrderStatus({ status }: { status: string }) {
  const { t } = useTranslation()
  const known = orderStatuses.find((value) => value === status)
  return (
    <span className="whitespace-nowrap">
      <StatusDot tone={known ? statusTones[known] : 'off'}>
        <span className="text-text">{t(known ? `merchant.overview.statuses.${known}` : 'merchant.overview.unknownStatus')}</span>
      </StatusDot>
    </span>
  )
}

export function OrderChannel({ channel }: { channel: string | null }) {
  const { t } = useTranslation()
  const known = orderChannels.find((value) => value === channel)
  if (!known) return t('merchant.overview.channels.unassigned')
  return <ChannelLabel channel={known}>{t(`merchant.overview.channels.${known}`)}</ChannelLabel>
}

export function OrderTime({ value }: { value: string | null }) {
  const { i18n } = useTranslation()
  if (!value || Number.isNaN(Date.parse(value))) return <>—</>
  return <time dateTime={value}>{formatOrderTime(value, i18n.resolvedLanguage)}</time>
}

export function formatOrderTime(value: string | null, language: string | undefined) {
  if (!value || Number.isNaN(Date.parse(value))) return '—'
  return new Intl.DateTimeFormat(language, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

type Order = components['schemas']['PortalOrderDTO']

export function OrderRows({ orders, filters, role = 'merchant' }: { orders: Order[]; filters: OrderFilters; role?: 'merchant' | 'console' }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  const amount = (order: Order) => money(order.actual_cents ?? order.original_cents, language)
  const link = (order: Order) => (
    <Link
      to={role === 'console' ? '/console/orders/$tradeNo' : '/orders/$tradeNo'}
      params={{ tradeNo: order.trade_no }}
      search={filters}
      aria-label={order.trade_no}
      className="text-text underline decoration-border-strong underline-offset-4 hover:text-accent-text hover:decoration-current"
    >
      <Identifier value={order.trade_no} />
    </Link>
  )
  return (
    <>
      <Table caption={t('merchant.orders.title')} minWidth={720} className="hidden md:block">
        <thead>
          <tr>
            <Th>{t('merchant.overview.tradeNo')}</Th>
            {role === 'console' && <Th>{t('manage.merchant')}</Th>}
            <Th>{t('merchant.overview.product')}</Th>
            <Th>{t('merchant.overview.channel')}</Th>
            <Th align="right">{t('merchant.overview.amount')}</Th>
            <Th>{t('merchant.overview.status')}</Th>
            <Th align="right">{t('merchant.overview.createdAt')}</Th>
          </tr>
        </thead>
        <tbody>
          {orders.map((order) => (
            <tr key={order.trade_no} className="transition-colors hover:bg-hover/50">
              <Td className="w-[30%] max-w-[260px]">
                {link(order)}
                <div className="mt-1 text-text-3"><Identifier value={order.out_trade_no} className="text-caption" /></div>
              </Td>
              {role === 'console' && <Td className="max-w-[160px]"><RecordLink to={'/console/merchants/' + order.merchant_id}>{order.merchant_username}</RecordLink></Td>}
              <Td className="max-w-[220px] break-words">{order.name}</Td>
              <Td className="whitespace-nowrap text-text-2"><OrderChannel channel={order.channel} /></Td>
              <Td align="right" className="whitespace-nowrap font-medium">{amount(order)}</Td>
              <Td><OrderStatus status={order.status} /></Td>
              <Td align="right" className="whitespace-nowrap text-[13px] text-text-2"><OrderTime value={order.created_at} /></Td>
            </tr>
          ))}
        </tbody>
      </Table>
      <ul aria-label={t('merchant.orders.cards')} className="m-0 flex list-none flex-col gap-3 p-0 md:hidden">
        {orders.map((order) => (
          <li key={order.trade_no} className="rounded-card border-[0.5px] border-border bg-surface p-4">
            <div className="flex items-start justify-between gap-3">
              <p className="m-0 min-w-0 break-words text-[15px] leading-snug">{order.name}</p>
              <span className="shrink-0 font-serif text-[1.375rem] leading-none tabular-nums">{amount(order)}</span>
            </div>
            <div className="mt-2.5">{link(order)}</div>
            {role === 'console' && <p className="m-0 mt-0.5 text-caption text-text-3 [overflow-wrap:anywhere]">
              {t('manage.merchant')} · <RecordLink to={'/console/merchants/' + order.merchant_id}>{order.merchant_username}</RecordLink>
            </p>}
            <p className="m-0 mt-0.5 font-mono text-caption text-text-3 [overflow-wrap:anywhere]">
              {t('merchant.orders.outTradeNo')} · <Identifier value={order.out_trade_no} className="text-caption" />
            </p>
            <div className="mt-3 flex flex-wrap items-center justify-between gap-x-3 gap-y-1 border-t-[0.5px] border-border pt-3 text-caption text-text-2">
              <OrderStatus status={order.status} />
              <span className="flex flex-wrap gap-x-2">
                <OrderChannel channel={order.channel} />
                <span aria-hidden="true">·</span>
                <OrderTime value={order.created_at} />
              </span>
            </div>
          </li>
        ))}
      </ul>
    </>
  )
}
