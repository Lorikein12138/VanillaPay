import { Link } from '@tanstack/react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import type { components } from '@/api/schema'
import { OrderActionDialog } from './OrderActionDialog'
import { availableOrderActions, orderErrorKey, type OrderAction, type OrderFilters } from './orders-data'
import { money } from './overview-data'
import { Skeleton } from './OverviewCards'
import { orderButton, OrderChannel, OrderStatus, OrderTime } from './OrderParts'

export function OrderDetail({ name, tradeNo, filters }: { name: string; tradeNo: string; filters: OrderFilters }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  const client = useQueryClient()
  const query = useQuery({ queryKey: ['merchant', name, 'order', tradeNo], queryFn: () => api.order(tradeNo) })
  const [confirmation, setConfirmation] = useState<{ action: OrderAction; key: string; detail: components['schemas']['OrderDetail'] }>()
  const [notice, setNotice] = useState<OrderAction>()
  const detail = query.data
  const order = detail?.order
  const refund = detail?.refund
  const back = <Link to="/orders" search={filters} className="inline-flex min-h-11 items-center text-label text-accent-text underline-offset-4 hover:underline">← {t('merchant.orders.back')}</Link>
  if (query.isPending) return <div className="mx-auto max-w-[1112px]" aria-busy="true">{back}<Skeleton className="mt-5 h-96 w-full" /></div>
  if (!detail || !order) return <div className="mx-auto max-w-[1112px] space-y-5">{back}<h1 className="font-serif text-3xl">{t(query.error instanceof APIError && query.error.status === 404 ? 'merchant.orders.notFound' : 'merchant.orders.detail')}</h1><p role="alert" className="text-label text-text-2">{t(orderErrorKey(query.error))}</p>{!(query.error instanceof APIError && query.error.status === 404) && <button type="button" className={orderButton} onClick={() => { void query.refetch() }}>{t('error.retry')}</button>}</div>
  async function completed() {
    await Promise.all([client.invalidateQueries({ queryKey: ['merchant', name, 'order', tradeNo] }), client.invalidateQueries({ queryKey: ['merchant', name, 'orders'] })])
    setNotice(confirmation?.action)
    setConfirmation(undefined)
  }
  return <div className="mx-auto flex max-w-[1112px] flex-col gap-8">
    <div>{back}<div className="mt-4 flex flex-wrap items-start justify-between gap-4"><div className="min-w-0"><h1 className="font-serif text-4xl font-[330]">{t('merchant.orders.detail')}</h1><p className="mt-3 break-all font-mono text-label text-text-2">{order.trade_no}</p></div><OrderStatus status={order.status} /></div></div>
    {notice && <p role="status" className="rounded-control border-[0.5px] border-border bg-muted px-4 py-3 text-label">{t(`merchant.orders.success.${notice}`)}</p>}
    {query.isError && <div role="alert" className="text-label text-text-2">{t(orderErrorKey(query.error))} <button className={orderButton} onClick={() => { void query.refetch() }}>{t('error.retry')}</button></div>}
    <section aria-label={t('merchant.orders.operations')} className="flex flex-wrap gap-2">{availableOrderActions(detail).map((action) => <button type="button" key={action} className={orderButton} onClick={() => { setNotice(undefined); setConfirmation({ action, key: crypto.randomUUID(), detail }) }}>{t(`merchant.orders.actions.${action}`)}</button>)}</section>
    <section className="rounded-card border-[0.5px] border-border bg-surface p-5 sm:p-7"><h2 className="mb-6 font-serif text-heading">{t('merchant.orders.information')}</h2><dl className="grid gap-x-8 gap-y-6 sm:grid-cols-2 lg:grid-cols-3">
      <Field label={t('merchant.overview.product')}>{order.name}</Field><Field label={t('merchant.orders.outTradeNo')}>{order.out_trade_no}</Field><Field label={t('merchant.overview.channel')}><OrderChannel channel={order.channel} /></Field>
      <Field label={t('merchant.orders.originalAmount')}><span className="font-serif text-2xl tabular-nums">{money(order.original_cents, language)}</span></Field><Field label={t('merchant.orders.actualAmount')}><span className="font-serif text-2xl tabular-nums">{order.actual_cents === null ? '—' : money(order.actual_cents, language)}</span></Field><Field label={t('merchant.orders.callbackStatus')}>{t(`merchant.orders.callback.${order.callback_status}`, { defaultValue: order.callback_status })}</Field>
      <Field label={t('merchant.overview.createdAt')}><OrderTime value={order.created_at} /></Field><Field label={t('merchant.orders.paidAt')}><OrderTime value={order.paid_at} /></Field><Field label={t('merchant.orders.expiresAt')}><OrderTime value={order.expires_at} /></Field>
      <Field label={t('merchant.orders.purpose')}>{t(order.purpose === 'subscription' ? 'merchant.orders.subscription' : 'merchant.orders.external')}</Field><Field label={t('merchant.orders.protocol')}>{order.protocol}</Field><Field label={t('merchant.orders.productId')}>{order.product_id || '—'}</Field>
      <Field label={t('merchant.orders.manual')}>{t(order.is_manual ? 'merchant.orders.yes' : 'merchant.orders.no')}</Field><Field label={t('merchant.orders.manualReason')}>{order.manual_reason || '—'}</Field><Field label={t('merchant.orders.expireReason')}>{order.expire_reason || '—'}</Field>
      <Field label="notify_url">{detail.notify_url || '—'}</Field><Field label="return_url">{detail.return_url || '—'}</Field><Field label={t('merchant.orders.param')}>{order.param || '—'}</Field>
    </dl></section>
    <section className="rounded-card border-[0.5px] border-border bg-surface p-5 sm:p-7"><h2 className="mb-5 font-serif text-heading">{t('merchant.orders.refundInfo')}</h2>{refund ? <dl className="grid gap-x-8 gap-y-5 sm:grid-cols-2 lg:grid-cols-3">
      <Field label={t('merchant.orders.refundId')}>{refund.id}</Field><Field label={t('merchant.orders.refundStatus')}>{t(`merchant.orders.refundStatusValues.${refund.status}`, { defaultValue: refund.status })}</Field><Field label={t('merchant.orders.refundAmount')}>{money(refund.amount_cents, language)}</Field><Field label={t('merchant.orders.refundOutNo')}>{refund.out_refund_no || '—'}</Field><Field label={t('merchant.orders.reason')}>{refund.reason || '—'}</Field><Field label={t('merchant.orders.requestedAt')}><OrderTime value={refund.requested_at} /></Field><Field label={t('merchant.orders.handledAt')}><OrderTime value={refund.handled_at} /></Field><Field label={t('merchant.orders.handledBy')}>{refund.handled_by || '—'}</Field>
    </dl> : <p className="text-label text-text-3">{t('merchant.orders.noRefund')}</p>}{order.refunded_at && <p className="mt-5 text-label text-text-2">{t('merchant.orders.refundedAt')} · <OrderTime value={order.refunded_at} /></p>}</section>
    <section><h2 className="mb-5 font-serif text-heading">{t('merchant.orders.attempts')}</h2>{detail.callback_attempts?.length ? <ol className="space-y-4">{detail.callback_attempts.map((attempt) => <li key={attempt.id} className="rounded-card border-[0.5px] border-border bg-surface p-5">
      <h3 className="mb-4 text-label font-medium">{t('merchant.orders.attempt', { generation: attempt.generation, number: attempt.attempt_no })}</h3><dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"><Field label={t('merchant.orders.startedAt')}><OrderTime value={attempt.started_at} /></Field><Field label={t('merchant.orders.finishedAt')}><OrderTime value={attempt.finished_at} /></Field><Field label={t('merchant.orders.httpStatus')}>{attempt.http_status ?? '—'}</Field><Field label={t('merchant.orders.duration')}>{attempt.duration_ms === null ? '—' : `${attempt.duration_ms} ms`}</Field><Field label={t('merchant.orders.errorCode')}>{attempt.error_code || '—'}</Field><Field label={t('merchant.orders.nextAttempt')}><OrderTime value={attempt.next_attempt_at} /></Field><Field label={t('merchant.orders.target')}>{attempt.url_redacted}</Field></dl><pre className="mt-4 whitespace-pre-wrap break-all rounded-control bg-muted p-3 font-mono text-caption text-text-2">{attempt.response_excerpt || '—'}</pre>
    </li>)}</ol> : <p className="border-t-[0.5px] border-border py-6 text-label text-text-3">{t('merchant.orders.noAttempts')}</p>}</section>
    {confirmation && <OrderActionDialog key={confirmation.key} action={confirmation.action} idempotencyKey={confirmation.key} detail={confirmation.detail} onClose={() => setConfirmation(undefined)} onSuccess={completed} />}
  </div>
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return <div className="min-w-0"><dt className="mb-1.5 text-caption text-text-3">{label}</dt><dd className="whitespace-pre-wrap break-all text-label">{children}</dd></div>
}
