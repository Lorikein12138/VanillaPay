import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError } from '@/api/client'
import type { components } from '@/api/schema'
import { orderDetailQuery } from '@/app/portal-queries'
import { Button } from '@/components/ui/button'
import { CopyIdentifier } from '@/components/ui/copy-button'
import { DetailList } from '@/components/ui/detail-list'
import { BackLink, Card, Identifier, Page, PageHeader, SectionHeader, Skeleton, SubsectionTitle } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { RecordLink } from '@/components/ui/records'
import { cn } from '@/lib/utils'
import { OrderActionDialog } from './OrderActionDialog'
import { FormHint } from '@/components/ui/auth-screen'
import { availableOrderActions, orderErrorKey, type OrderAction, type OrderFilters } from './orders-data'
import { money } from './overview-data'
import { OrderChannel, OrderStatus, OrderTime } from './OrderParts'

type Detail = components['schemas']['OrderDetail']

export function OrderDetail({ name, tradeNo, filters, role = 'merchant' }: { role?: 'merchant' | 'console'; name: string; tradeNo: string; filters: OrderFilters }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const query = useQuery(orderDetailQuery(role, name, tradeNo))
  const [confirmation, setConfirmation] = useState<{ action: OrderAction; key: string; detail: Detail }>()
  const [notice, setNotice] = useState<OrderAction>()
  const replayLimitHintId = useId()
  const detail = query.data
  const back = <BackLink to={role === 'console' ? '/console/orders' : '/orders'} search={filters}>{t('merchant.orders.back')}</BackLink>
  if (query.isPending) {
    return (
      <Page className="gap-8" aria-busy="true">
        <div className="flex flex-col gap-4">{back}<Skeleton className="h-10 w-48" /><Skeleton className="h-4 w-72" /></div>
        <Skeleton className="h-28 w-full rounded-card" />
        <Skeleton className="h-80 w-full rounded-card" />
      </Page>
    )
  }
  if (!detail?.order) {
    const missing = query.error instanceof APIError && query.error.status === 404
    return (
      <Page className="gap-6">
        <PageHeader back={back} title={t(missing ? 'merchant.orders.notFound' : 'merchant.orders.detail')} />
        <Notice
          tone={missing ? 'info' : 'error'}
          action={!missing && <Button size="sm" onClick={() => { void query.refetch() }}>{t('error.retry')}</Button>}
        >
          {t(orderErrorKey(query.error))}
        </Notice>
      </Page>
    )
  }
  const order = detail.order
  const actions = role === 'console' ? [] : availableOrderActions(detail)
  const replayLimitReached = detail.callback_replays_used >= detail.callback_replay_limit
  async function completed() {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['merchant', name, 'order', tradeNo] }),
      client.invalidateQueries({ queryKey: ['merchant', name, 'orders'] }),
      client.invalidateQueries({ queryKey: ['merchant', name, 'stats'] }),
    ])
    setNotice(confirmation?.action)
    setConfirmation(undefined)
  }
  return (
    <Page className="gap-8">
      <PageHeader
        back={back}
        title={t('merchant.orders.detail')}
        meta={
          <CopyIdentifier value={order.trade_no} label={t('merchant.orders.copyTradeNo')} />
        }
        actions={actions.length > 0 && (
          <section aria-label={t('merchant.orders.operations')} className="flex flex-col items-start gap-1.5 sm:items-end">
            <div className="flex flex-wrap gap-2">
              {actions.map((action) => {
                const limited = action === 'replay' && replayLimitReached
                return (
                  <Button
                    key={action}
                    variant={action === 'reject' ? 'danger' : 'secondary'}
                    disabled={limited}
                    aria-describedby={limited ? replayLimitHintId : undefined}
                    onClick={() => { setNotice(undefined); setConfirmation({ action, key: crypto.randomUUID(), detail }) }}
                  >
                    {t(`merchant.orders.actions.${action}`)}
                  </Button>
                )
              })}
            </div>
            {actions.includes('replay') && replayLimitReached && (
              <div id={replayLimitHintId} className="sm:text-right">
                <FormHint>{t('merchant.orders.replayLimitReached', { used: detail.callback_replays_used, limit: detail.callback_replay_limit })}</FormHint>
              </div>
            )}
          </section>
        )}
      />
      {notice && (
        <Notice tone="success" onDismiss={() => setNotice(undefined)}>{t(`merchant.orders.success.${notice}`)}</Notice>
      )}
      {query.isError && (
        <Notice tone="error" action={<Button size="sm" onClick={() => { void query.refetch() }}>{t('error.retry')}</Button>}>
          {t(orderErrorKey(query.error))}
        </Notice>
      )}
      <Summary detail={detail} />
      <Card>
        <SectionHeader title={t('merchant.orders.information')} className="mb-2" />
        <OrderInformation detail={detail} merchant={role === 'console' && <RecordLink to={'/console/merchants/' + order.merchant_id}>{order.merchant_username}</RecordLink>} />
      </Card>
      <Card>
        <SectionHeader title={t('merchant.orders.refundInfo')} className="mb-2" />
        <RefundInformation detail={detail} />
      </Card>
      <section className="flex flex-col gap-4">
        <h2 className="m-0 font-serif text-heading">{t('merchant.orders.attempts')}</h2>
        <CallbackAttempts detail={detail} />
      </section>
      {confirmation && (
        <OrderActionDialog
          key={confirmation.key}
          action={confirmation.action}
          idempotencyKey={confirmation.key}
          detail={confirmation.detail}
          onClose={() => setConfirmation(undefined)}
          onSuccess={completed}
          onRefundConflict={async () => { await query.refetch() }}
        />
      )}
    </Page>
  )
}

/** Amount, status, channel and callback state: what a merchant checks first. */
function Summary({ detail }: { detail: Detail }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  const order = detail.order
  return (
    <Card className="grid grid-cols-2 gap-x-6 gap-y-5 p-5 sm:p-7 lg:grid-cols-[minmax(0,1.4fr)_repeat(3,minmax(0,1fr))]">
      <SummaryItem label={t(order.actual_cents === null ? 'merchant.orders.orderAmount' : 'merchant.orders.actualAmount')} className="col-span-2 lg:col-span-1">
        <span className="font-serif text-figure tabular-nums">{money(order.actual_cents ?? order.original_cents, language)}</span>
      </SummaryItem>
      <SummaryItem label={t('merchant.overview.status')}><OrderStatus status={order.status} /></SummaryItem>
      <SummaryItem label={t('merchant.overview.channel')}><OrderChannel channel={order.channel} /></SummaryItem>
      <SummaryItem label={t('merchant.orders.callbackStatus')} className="col-span-2 sm:col-span-1">
        {t(`merchant.orders.callback.${order.callback_status}`, { defaultValue: order.callback_status })}
      </SummaryItem>
    </Card>
  )
}

function SummaryItem({ label, className, children }: { label: string; className?: string; children: ReactNode }) {
  return (
    <div className={cn('flex min-w-0 flex-col gap-1.5', className)}>
      <span className="text-caption text-text-3">{label}</span>
      <div className="text-[15px]">{children}</div>
    </div>
  )
}

function OrderInformation({ detail, merchant }: { detail: Detail; merchant?: ReactNode }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  const order = detail.order
  // The headline amount is shown above; repeat the original only when it differs.
  const adjusted = order.actual_cents !== null && order.actual_cents !== order.original_cents
  return (
    <DetailList
      items={[
        { label: t('manage.merchant'), value: merchant || null },
        { label: t('merchant.overview.product'), value: order.name },
        { label: t('merchant.orders.outTradeNo'), value: <Identifier value={order.out_trade_no} /> },
        { label: t('merchant.orders.originalAmount'), value: adjusted ? money(order.original_cents, language) : null },
        { label: t('merchant.overview.createdAt'), value: <OrderTime value={order.created_at} /> },
        { label: t('merchant.orders.paidAt'), value: order.paid_at && <OrderTime value={order.paid_at} /> },
        { label: t('merchant.orders.expiresAt'), value: order.expires_at && <OrderTime value={order.expires_at} /> },
        { label: t('merchant.orders.purpose'), value: t(order.purpose === 'subscription' ? 'merchant.orders.subscription' : 'merchant.orders.external') },
        { label: t('merchant.orders.protocol'), value: order.protocol },
        { label: t('merchant.orders.productId'), value: order.product_id, mono: true },
        { label: t('merchant.orders.manual'), value: order.is_manual ? t('merchant.orders.yes') : null },
        { label: t('merchant.orders.manualReason'), value: order.manual_reason },
        { label: t('merchant.orders.expireReason'), value: order.expire_reason && t(`merchant.orders.expireReasons.${order.expire_reason}`, { defaultValue: order.expire_reason }) },
        { label: t('merchant.orders.param'), value: order.param, mono: true },
        { label: t('merchant.orders.notifyUrl'), value: detail.notify_url, mono: true, wide: true },
        { label: t('merchant.orders.returnUrl'), value: detail.return_url, mono: true, wide: true },
      ]}
    />
  )
}

function RefundInformation({ detail }: { detail: Detail }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  const { refund, order } = detail
  return (
    <>
      {refund ? (
        <DetailList
          items={[
            { label: t('merchant.orders.refundStatus'), value: t(`merchant.orders.refundStatusValues.${refund.status}`, { defaultValue: refund.status }) },
            { label: t('merchant.orders.refundAmount'), value: money(refund.amount_cents, language) },
            { label: t('merchant.orders.requestedAt'), value: <OrderTime value={refund.requested_at} /> },
            { label: t('merchant.orders.refundId'), value: refund.id, mono: true },
            { label: t('merchant.orders.refundOutNo'), value: refund.out_refund_no, mono: true },
            { label: t('merchant.orders.handledAt'), value: refund.handled_at && <OrderTime value={refund.handled_at} /> },
            { label: t('merchant.orders.handledBy'), value: refund.handled_by },
            { label: t('merchant.orders.reason'), value: refund.reason, wide: true },
          ]}
        />
      ) : (
        <p className="m-0 border-t-[0.5px] border-border pt-3.5 text-label text-text-3">{t('merchant.orders.noRefund')}</p>
      )}
      {order.refunded_at && (
        <p className="m-0 mt-4 text-label text-text-2">
          {t('merchant.orders.refundedAt')} · <OrderTime value={order.refunded_at} />
        </p>
      )}
    </>
  )
}

function CallbackAttempts({ detail }: { detail: Detail }) {
  const { t } = useTranslation()
  const attempts = detail.callback_attempts ?? []
  if (!attempts.length) {
    return <p className="m-0 border-t-[0.5px] border-border py-6 text-label text-text-3">{t('merchant.orders.noAttempts')}</p>
  }
  return (
    <ol className="m-0 flex list-none flex-col gap-3 p-0">
      {attempts.map((attempt) => {
        const ok = attempt.http_status !== null && attempt.http_status >= 200 && attempt.http_status < 300 && !attempt.error_code
        return (
          <li key={attempt.id} className="rounded-card border-[0.5px] border-border bg-surface p-5 sm:px-7">
            <SubsectionTitle className="mb-2 flex items-center gap-2.5">
              <span aria-hidden="true" className={cn('size-[7px] shrink-0 rounded-full', ok ? 'bg-dot-ok' : attempt.finished_at ? 'bg-danger' : 'bg-dot-wait')} />
              {t('merchant.orders.attempt', { generation: attempt.generation, number: attempt.attempt_no })}
            </SubsectionTitle>
            <DetailList
              items={[
                { label: t('merchant.orders.startedAt'), value: <OrderTime value={attempt.started_at} /> },
                { label: t('merchant.orders.finishedAt'), value: attempt.finished_at && <OrderTime value={attempt.finished_at} /> },
                { label: t('merchant.orders.duration'), value: attempt.duration_ms === null ? null : `${attempt.duration_ms} ms` },
                { label: t('merchant.orders.httpStatus'), value: attempt.http_status, mono: true },
                { label: t('merchant.orders.errorCode'), value: attempt.error_code, mono: true },
                { label: t('merchant.orders.nextAttempt'), value: attempt.next_attempt_at && <OrderTime value={attempt.next_attempt_at} /> },
                { label: t('merchant.orders.target'), value: attempt.url_redacted, mono: true, wide: true },
              ]}
            />
            {attempt.response_excerpt && (
              <div className="mt-1 border-t-[0.5px] border-border pt-3.5">
                <p className="m-0 mb-1.5 text-caption text-text-3">{t('merchant.orders.responseExcerpt')}</p>
                <pre className="m-0 max-h-48 overflow-auto whitespace-pre-wrap rounded-control bg-muted p-3 font-mono text-caption leading-relaxed text-text-2 [overflow-wrap:anywhere]">
                  {attempt.response_excerpt}
                </pre>
              </div>
            )}
          </li>
        )
      })}
    </ol>
  )
}
