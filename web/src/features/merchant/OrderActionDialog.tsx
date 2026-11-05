import { Dialog } from 'radix-ui'
import { useId, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { components } from '@/api/schema'
import { api } from '@/api/client'
import { FormError } from '@/components/FormError'
import { orderButton, orderInput, orderPrimary } from './OrderParts'
import { orderChannels, orderErrorKey, validReason, type OrderAction } from './orders-data'

export function OrderActionDialog({ action, detail, idempotencyKey, onClose, onSuccess }: {
  action: OrderAction; detail: components['schemas']['OrderDetail']; idempotencyKey: string; onClose: () => void; onSuccess: () => Promise<void>
}) {
  const { t } = useTranslation()
  const reasonId = useId()
  const [reason, setReason] = useState('')
  const [channel, setChannel] = useState(detail.order.channel ?? '')
  const [confirm, setConfirm] = useState(false)
  const [attempted, setAttempted] = useState(false)
  const [busy, setBusy] = useState(false)
  const submitting = useRef(false)
  const [error, setError] = useState<unknown>()
  const isRefund = action === 'refund' || action === 'resolve'
  const valid = (action === 'replay' || validReason(reason, action)) && (!isRefund || confirm) && (action !== 'supplement' || !!channel)
  async function submit() {
    if (submitting.current || !valid) return
    submitting.current = true
    setBusy(true)
    setAttempted(true)
    setError(undefined)
    try {
      const trade = detail.order.trade_no
      if (action === 'supplement') await api.supplement(trade, { reason: reason.trim(), ...(channel ? { channel } : {}) }, idempotencyKey)
      else if (action === 'refund') await api.refund(trade, { confirm, reason: reason.trim() }, idempotencyKey)
      else if (action === 'replay') await api.replayCallback(trade, idempotencyKey)
      else if (detail.refund) await api.resolveRefund(detail.refund.id, { action: action === 'resolve' ? 'mark_refunded' : 'reject', confirm, reason: reason.trim() }, idempotencyKey)
      else throw new Error('refund no longer available')
      await onSuccess()
    } catch (problem) { setError(problem) }
    finally { submitting.current = false; setBusy(false) }
  }
  return <Dialog.Root open onOpenChange={(open) => { if (!open && !submitting.current) onClose() }}><Dialog.Portal>
    <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
    <Dialog.Content className="fixed inset-x-4 top-[12dvh] z-50 mx-auto max-h-[80dvh] max-w-[480px] overflow-y-auto rounded-card border-[0.5px] border-border-strong bg-surface p-5 shadow-xl sm:p-7" onEscapeKeyDown={(event) => { if (submitting.current) event.preventDefault() }} onPointerDownOutside={(event) => { if (submitting.current) event.preventDefault() }}>
      <Dialog.Title className="font-serif text-2xl">{t(`merchant.orders.actions.${action}`)}</Dialog.Title>
      <Dialog.Description className="mt-3 text-label leading-relaxed text-text-2">{t(`merchant.orders.confirm.${action}`)}</Dialog.Description>
      <p className="my-4 break-all rounded-control bg-muted px-3 py-2 font-mono text-caption">{detail.order.trade_no}</p>
      <form onSubmit={(event) => { event.preventDefault(); void submit() }} className="space-y-4">
        {action === 'supplement' && <div className="space-y-2 text-label"><label htmlFor={`${reasonId}-channel`}>{t('merchant.orders.supplementChannel')}</label><select id={`${reasonId}-channel`} className={orderInput} value={channel} disabled={busy || attempted || !!detail.order.channel} onChange={(event) => setChannel(event.target.value)}><option value="">{t('merchant.orders.selectChannel')}</option>{orderChannels.map((value) => <option key={value} value={value}>{t(`merchant.overview.channels.${value}`)}</option>)}</select></div>}
        {action !== 'replay' && <div className="space-y-2 text-label"><label htmlFor={reasonId}>{t('merchant.orders.reason')}</label><textarea id={reasonId} aria-describedby={`${reasonId}-hint`} required rows={3} maxLength={500} className={orderInput} value={reason} readOnly={busy || attempted} onChange={(event) => setReason(event.target.value)} /><p id={`${reasonId}-hint`} className="text-caption text-text-3">{t(action === 'supplement' ? 'merchant.orders.reasonHint' : 'merchant.orders.reasonBytesHint')}</p></div>}
        {isRefund && <label className="flex items-start gap-2 text-label"><input type="checkbox" className="mt-1 size-4 shrink-0 accent-accent" checked={confirm} disabled={busy || attempted} onChange={(event) => setConfirm(event.target.checked)} /><span>{t('merchant.orders.refundConfirmed')}</span></label>}
        {error != null && <FormError>{t(orderErrorKey(error))}</FormError>}
        {attempted && error != null && <p className="text-caption text-text-3">{t('merchant.orders.retryHint')}</p>}
        <div className="flex flex-wrap justify-end gap-2 pt-2"><button type="button" className={orderButton} disabled={busy} onClick={onClose}>{t('merchant.orders.cancel')}</button><button type="submit" className={orderPrimary} disabled={busy || !valid}>{t(busy ? 'merchant.orders.submitting' : `merchant.orders.submit.${action}`)}</button></div>
      </form>
    </Dialog.Content>
  </Dialog.Portal></Dialog.Root>
}
