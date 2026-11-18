import { Select, SelectOption } from '@/components/ui/select'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { components } from '@/api/schema'
import { APIError, api } from '@/api/client'
import { FormError } from '@/components/FormError'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Field } from '@/components/ui/field'
import { Checkbox, Textarea } from '@/components/ui/input'
import { canSupplementOrder, orderChannels, orderErrorKey, validReason, type OrderAction } from './orders-data'
import { money } from './overview-data'
import { OrderChannel, OrderStatus } from './OrderParts'
import { invalidateOrderViews, loadLatestOrder } from './useOrderActions'

type Detail = components['schemas']['OrderDetail']
type SupplementRequest = components['schemas']['PaymentSupplementRequest']
type SupplementMessage = 'merchant.orders.errors.supplementChanged' | 'merchant.orders.errors.supplementUnavailable' | 'merchant.orders.errors.supplementRefreshFailed' | 'merchant.orders.errors.supplementAmountChanged'

export function OrderActionDialog({ action, detail, name, idempotencyKey, onClose, onSuccess, onRefundConflict }: {
  action: OrderAction; detail: Detail; name: string; idempotencyKey: string
  onClose: () => void; onSuccess: () => Promise<void>; onRefundConflict?: () => Promise<void>
}) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient()
  const formId = useId()
  const reasonId = `${formId}-reason`
  const [supplementDetail, setSupplementDetail] = useState(detail)
  const [reason, setReason] = useState('')
  const [selectedChannel, setChannel] = useState('')
  const [confirm, setConfirm] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [supplementRetry, setSupplementRetry] = useState<SupplementRequest>()
  const [busy, setBusy] = useState(false)
  const submitting = useRef(false)
  const [error, setError] = useState<unknown>()
  const [supplementMessage, setSupplementMessage] = useState<SupplementMessage>()
  const supplementAlert = useRef<HTMLDivElement>(null)
  const [refreshFailed, setRefreshFailed] = useState(false)
  const refundHandled = isRefundHandled(error)
  const isSupplement = action === 'supplement'
  const isRefund = action === 'refund' || action === 'resolve'
  const order = isSupplement ? supplementDetail.order : detail.order
  const displayedAmount = order.actual_cents ?? order.original_cents
  const channel = order.channel ?? selectedChannel
  const allowedChannels = supplementDetail.allowed_channels_snapshot
  const inputsFrozen = attempted || !!supplementRetry
  const valid = !refundHandled && (action === 'replay' || validReason(reason, action)) && (!isRefund || confirm)
    && (!isSupplement || (!refreshFailed && canSupplementOrder(order) && allowedChannels.includes(channel)))

  useEffect(() => {
    if (supplementMessage) supplementAlert.current?.scrollIntoView?.({ block: 'nearest' })
  }, [supplementMessage])

  function applyLatest(latest: Detail, changed: SupplementMessage = 'merchant.orders.errors.supplementChanged') {
    setSupplementDetail(latest)
    if (latest.order.channel !== order.channel || !latest.allowed_channels_snapshot.includes(selectedChannel)) {
      setChannel('')
    }
    setSupplementMessage(canSupplementOrder(latest.order) ? changed : 'merchant.orders.errors.supplementUnavailable')
  }

  async function submitSupplement() {
    if (!supplementRetry) {
      const latest = await loadLatestOrder(client, name, order.trade_no)
      const next = latest.order
      if (!canSupplementOrder(next) || next.status !== order.status || next.channel !== order.channel ||
        next.actual_cents !== order.actual_cents || next.original_cents !== order.original_cents ||
        orderChannels.some((value) => allowedChannels.includes(value) !== latest.allowed_channels_snapshot.includes(value))) {
        applyLatest(latest)
        await invalidateOrderViews(client, name, order.trade_no, 'none')
        return
      }
    }
    const request = supplementRetry ?? { reason: reason.trim(), channel, expected_amount_cents: displayedAmount }
    try {
      await api.supplement(order.trade_no, request, idempotencyKey)
    } catch (problem) {
      const code = problem instanceof APIError ? problem.problem?.code : undefined
      if (code === 'CHANNEL_LOCKED' || code === 'ORDER_NOT_PAYABLE' || code === 'ORDER_AMOUNT_CHANGED') {
        setSupplementRetry(undefined)
        let detailRefetch: 'active' | 'none' = 'active'
        try {
          const latest = await loadLatestOrder(client, name, order.trade_no)
          applyLatest(latest, code === 'ORDER_AMOUNT_CHANGED' ? 'merchant.orders.errors.supplementAmountChanged' : undefined)
          detailRefetch = 'none'
        } catch {
          setRefreshFailed(true)
          setSupplementMessage('merchant.orders.errors.supplementRefreshFailed')
        }
        await invalidateOrderViews(client, name, order.trade_no, detailRefetch)
        return
      }
      // An unknown POST outcome must retry the same intent, even if a GET would now show it as paid.
      if (!(problem instanceof APIError) || problem.status === 0 || problem.status >= 500) setSupplementRetry(request)
      throw problem
    }
    await onSuccess()
  }

  async function submit() {
    if (submitting.current || !valid) return
    submitting.current = true
    setBusy(true)
    if (!isSupplement) setAttempted(true)
    setError(undefined)
    setSupplementMessage(undefined)
    try {
      const trade = order.trade_no
      if (isSupplement) { await submitSupplement(); return }
      if (action === 'refund') await api.refund(trade, { confirm, reason: reason.trim() }, idempotencyKey)
      else if (action === 'replay') await api.replayCallback(trade, idempotencyKey)
      else if (detail.refund) await api.resolveRefund(detail.refund.id, { action: action === 'resolve' ? 'mark_refunded' : 'reject', confirm, reason: reason.trim() }, idempotencyKey)
      else throw new Error('refund no longer available')
      await onSuccess()
    } catch (problem) {
      setError(problem)
      if (isRefundHandled(problem)) await onRefundConflict?.()
    }
    finally { submitting.current = false; setBusy(false) }
  }
  return (
    <Dialog
      onClose={onClose}
      busy={busy}
      title={t(`merchant.orders.actions.${action}`)}
      description={t(`merchant.orders.confirm.${action}`)}
      footer={
        <>
          <Button disabled={busy} onClick={onClose}>{t('merchant.orders.cancel')}</Button>
          <Button type="submit" form={formId} variant={action === 'reject' ? 'secondary' : 'primary'} loading={busy} disabled={!valid}>
            {t(busy ? 'merchant.orders.submitting' : `merchant.orders.submit.${action}`)}
          </Button>
        </>
      }
    >
      <form id={formId} onSubmit={(event) => { event.preventDefault(); void submit() }} className="flex flex-col gap-5">
        {supplementMessage && <div ref={supplementAlert}><FormError>{t(supplementMessage)}</FormError></div>}
        <div className="flex items-center justify-between gap-4 rounded-control bg-muted px-4 py-3">
          <div className="flex min-w-0 flex-col gap-1">
            <span className="font-mono text-[13px] text-text-2 [overflow-wrap:anywhere]">{order.trade_no}</span>
            {isSupplement && <OrderStatus status={order.status} />}
          </div>
          <span className="shrink-0 font-serif text-[1.25rem] tabular-nums">
            {money(displayedAmount, i18n.resolvedLanguage ?? 'zh-CN')}
          </span>
        </div>
        {isSupplement && (order.channel ? (
          <dl className="m-0 flex flex-col gap-1.5">
            <dt className="text-label font-medium">{t('merchant.orders.supplementChannel')}</dt>
            <dd className="m-0"><OrderChannel channel={order.channel} /></dd>
          </dl>
        ) : (
          <Field label={t('merchant.orders.supplementChannel')} htmlFor={`${formId}-channel`}>
            <Select
              id={`${formId}-channel`}
              value={channel}
              disabled={busy || inputsFrozen || refreshFailed || !canSupplementOrder(order)}
              onValueChange={setChannel}
            >
              <SelectOption value="">{t('merchant.orders.selectChannel')}</SelectOption>
              {orderChannels.map((value) => (
                <SelectOption key={value} value={value} disabled={!allowedChannels.includes(value)}>{t(`merchant.overview.channels.${value}`)}</SelectOption>
              ))}
            </Select>
          </Field>
        ))}
        {action !== 'replay' && (
          <Field
            label={t('merchant.orders.reason')}
            htmlFor={reasonId}
            hint={t(action === 'supplement' ? 'merchant.orders.reasonHint' : 'merchant.orders.reasonBytesHint')}
          >
            <Textarea
              id={reasonId}
              aria-describedby={`${reasonId}-hint`}
              required
              maxLength={500}
              value={reason}
              readOnly={busy || inputsFrozen}
              onChange={(event) => setReason(event.target.value)}
            />
          </Field>
        )}
        {isRefund && (
          <Checkbox checked={confirm} disabled={busy || attempted} onChange={(event) => setConfirm(event.target.checked)}>
            {t('merchant.orders.refundConfirmed')}
          </Checkbox>
        )}
        {error != null && (
          <div className="flex flex-col gap-1.5">
            <FormError>{t(orderErrorKey(error))}</FormError>
            {inputsFrozen && !refundHandled && <p className="m-0 pl-6 text-caption text-text-3">{t('merchant.orders.retryHint')}</p>}
          </div>
        )}
      </form>
    </Dialog>
  )
}

function isRefundHandled(error: unknown) {
  return error instanceof APIError && error.problem?.code === 'REFUND_ALREADY_HANDLED'
}
