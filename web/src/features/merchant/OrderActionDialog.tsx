import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { components } from '@/api/schema'
import { APIError, api } from '@/api/client'
import { FormError } from '@/components/FormError'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Field } from '@/components/ui/field'
import { Checkbox, Select, Textarea } from '@/components/ui/input'
import { orderChannels, orderErrorKey, validReason, type OrderAction } from './orders-data'
import { money } from './overview-data'

export function OrderActionDialog({ action, detail, idempotencyKey, onClose, onSuccess, onRefundConflict }: {
  action: OrderAction; detail: components['schemas']['OrderDetail']; idempotencyKey: string
  onClose: () => void; onSuccess: () => Promise<void>; onRefundConflict: () => Promise<void>
}) {
  const { t, i18n } = useTranslation()
  const formId = useId()
  const reasonId = `${formId}-reason`
  const [reason, setReason] = useState('')
  const [channel, setChannel] = useState(detail.order.channel ?? '')
  const [confirm, setConfirm] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [busy, setBusy] = useState(false)
  const submitting = useRef(false)
  const [error, setError] = useState<unknown>()
  const refundHandled = isRefundHandled(error)
  const isRefund = action === 'refund' || action === 'resolve'
  const allowedChannels = detail.allowed_channels_snapshot
  const valid = !refundHandled && (action === 'replay' || validReason(reason, action)) && (!isRefund || confirm)
    && (action !== 'supplement' || allowedChannels.includes(channel))
  const order = detail.order
  async function submit() {
    if (submitting.current || !valid) return
    submitting.current = true
    setBusy(true)
    setAttempted(true)
    setError(undefined)
    try {
      const trade = order.trade_no
      if (action === 'supplement') await api.supplement(trade, { reason: reason.trim(), ...(channel ? { channel } : {}) }, idempotencyKey)
      else if (action === 'refund') await api.refund(trade, { confirm, reason: reason.trim() }, idempotencyKey)
      else if (action === 'replay') await api.replayCallback(trade, idempotencyKey)
      else if (detail.refund) await api.resolveRefund(detail.refund.id, { action: action === 'resolve' ? 'mark_refunded' : 'reject', confirm, reason: reason.trim() }, idempotencyKey)
      else throw new Error('refund no longer available')
      await onSuccess()
    } catch (problem) {
      setError(problem)
      if (isRefundHandled(problem)) await onRefundConflict()
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
        <div className="flex items-center justify-between gap-4 rounded-control bg-muted px-4 py-3">
          <span className="min-w-0 font-mono text-[13px] text-text-2 [overflow-wrap:anywhere]">{order.trade_no}</span>
          <span className="shrink-0 font-serif text-[1.25rem] tabular-nums">
            {money(order.actual_cents ?? order.original_cents, i18n.resolvedLanguage ?? 'zh-CN')}
          </span>
        </div>
        {action === 'supplement' && (
          <Field label={t('merchant.orders.supplementChannel')} htmlFor={`${formId}-channel`}>
            <Select
              id={`${formId}-channel`}
              value={channel}
              disabled={busy || attempted || !!order.channel}
              onChange={(event) => setChannel(event.target.value)}
            >
              <option value="">{t('merchant.orders.selectChannel')}</option>
              {orderChannels.map((value) => (
                <option key={value} value={value} disabled={!allowedChannels.includes(value)}>{t(`merchant.overview.channels.${value}`)}</option>
              ))}
            </Select>
          </Field>
        )}
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
              readOnly={busy || attempted}
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
            {attempted && !refundHandled && <p className="m-0 pl-6 text-caption text-text-3">{t('merchant.orders.retryHint')}</p>}
          </div>
        )}
      </form>
    </Dialog>
  )
}

function isRefundHandled(error: unknown) {
  return error instanceof APIError && error.problem?.code === 'REFUND_ALREADY_HANDLED'
}
