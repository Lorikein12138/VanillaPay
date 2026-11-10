import { subscriptionQuery } from '@/app/portal-queries'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import type { components } from '@/api/schema'
import { StatusDot } from '@/components/StatusDot'
import { Button, buttonVariants } from '@/components/ui/button'
import { CopyText } from '@/components/ui/copy-button'
import { formatCountdown, useCountdown } from '@/components/ui/countdown'
import { DetailList } from '@/components/ui/detail-list'
import { Dialog } from '@/components/ui/dialog'
import { Checkbox } from '@/components/ui/input'
import { Card, EmptyState, Page, PageHeader, SectionHeader, Skeleton } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { ChannelList } from '@/components/ui/payment'
import { ModuleError, useLocalAction } from './CollectionParts'
import { formatOrderTime, OrderTime } from './OrderParts'
import { money } from './overview-data'
import { invalidateSubscription, localized, purchaseKind, subscriptionError, type Plan, type Subscription } from './subscription-data'

export function SubscriptionPage({ name }: { name: string }) {
  const { t, i18n } = useTranslation(), client = useQueryClient()
  const subscription = useQuery({ ...subscriptionQuery(name), refetchOnWindowFocus: 'always', refetchInterval: (query) => query.state.data?.pending_purchase ? 15000 : false })
  const [selected, setSelected] = useState<{ plan: Plan; current: Subscription } | null>(null)
  const data = subscription.data, language = i18n.resolvedLanguage ?? 'zh-CN'
  const refresh = () => invalidateSubscription(client, name)
  return <Page>
    <PageHeader title={t('merchant.nav.subscription')} description={t('subscription.description')} />
    {subscription.isPending && <Skeleton className="h-64" />}
    {subscription.isError && <ModuleError error={subscription.error} retry={() => { void subscription.refetch() }} />}
    {data && <>
      {data.pending_purchase && <PendingPurchase key={data.pending_purchase.id} name={name} purchase={data.pending_purchase} onChanged={refresh} />}
      {!data.billing_ready && <Notice tone="info">{t('subscription.billingClosed')}</Notice>}
      <Card><SectionHeader title={t('subscription.current')} actions={<StatusDot tone={data.current.active ? 'ok' : 'off'}>{t(data.current.active ? 'subscription.active' : 'subscription.inactive')}</StatusDot>} />
        <DetailList columns={2} items={[
          { label: t('subscription.plan'), value: data.current.plan ? localized(data.current.plan.name, language) : t('subscription.noPlan') },
          { label: t('subscription.expires'), value: <OrderTime value={data.current.expires_at} /> },
          { label: t('subscription.source'), value: t(`subscription.sources.${data.current.source === 'manual' ? 'manual' : data.current.source === 'purchase' ? 'purchase' : 'none'}`) },
          { label: t('subscription.channels'), value: <Channels channels={data.current.channels} /> },
        ]} />
      </Card>
      <section aria-label={t('subscription.available')}><SectionHeader title={t('subscription.available')} />
        {data.pending_purchase && <div className="mb-5"><Notice tone="info">{t('subscription.pendingBlocks')}</Notice></div>}
        {data.plans?.length ? <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">{data.plans.map((plan) => {
          const kind = purchaseKind(data.current, plan)
          return <Card key={plan.id} aria-label={localized(plan.name, language)} className="flex flex-col gap-5">
            <SectionHeader className="mb-0" title={localized(plan.name, language)} actions={data.current.plan?.id === plan.id && <StatusDot tone="info">{t('subscription.currentPlan')}</StatusDot>} />
            <p className="m-0 font-serif text-[2rem]">{money(plan.price_cents, language)}<span className="ml-2 font-sans text-caption text-text-3">{t('subscription.duration', { count: plan.duration_days })}</span></p>
            <div className="text-label text-text-2"><Channels channels={plan.channels} /></div>
            <p className="m-0 text-caption text-text-3">{plan.purchase_limit == null ? t('subscription.unlimited') : t('subscription.remaining', { count: plan.remaining_count ?? 0 })}</p>
            <div className="mt-auto border-t-[0.5px] border-border pt-5"><Button className="w-full" variant="primary" disabled={!data.billing_ready || !!data.pending_purchase || plan.deleted || plan.remaining_count === 0} onClick={() => setSelected({ plan, current: data.current })}>{t(`subscription.buy.${kind}`)}</Button></div>
          </Card>
        })}</div> : <EmptyState title={t('subscription.noPlans')} />}
      </section>
    </>}
    {selected && <PurchaseDialog plan={selected.plan} current={selected.current} onClose={() => setSelected(null)} onChanged={refresh} />}
  </Page>
}

function Channels({ channels }: { channels: string[] | null }) {
  const { t } = useTranslation()
  return channels?.length ? <ChannelList channels={channels} name={(channel) => t(`merchant.overview.channels.${channel}`, { defaultValue: channel })} /> : <>—</>
}

export function PurchaseDialog({ plan, current, onClose, onChanged }: { plan: Plan; current: Subscription; onClose: () => void; onChanged: () => Promise<void> }) {
  const { t, i18n } = useTranslation(), action = useLocalAction()
  // Both revisions and the UUID belong to this confirmation, including retries.
  const [key] = useState(() => crypto.randomUUID()), [confirmed, setConfirmed] = useState(false), [stale, setStale] = useState(false)
  const kind = purchaseKind(current, plan)
  async function submit() {
    if (stale || (kind === 'switch' && !confirmed)) return
    await action.run(async () => {
      try {
        await api.purchase({ plan_id: plan.id, plan_revision: plan.revision, confirm_replace: kind === 'switch', expected_subscription_revision: current.revision }, key)
        await onChanged()
        if (action.active.current) onClose()
      } catch (error) {
        if (error instanceof APIError && error.status === 409) {
          if (action.active.current) setStale(true)
          await onChanged()
        }
        throw error
      }
    })
  }
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  return <Dialog title={t(`subscription.confirm.${kind}`)} description={t(`subscription.explain.${kind}`, { count: plan.duration_days })} onClose={onClose} busy={action.busy} footer={<Button variant="primary" loading={action.busy} disabled={stale || (kind === 'switch' && !confirmed)} onClick={() => { void submit() }}>{t('subscription.createPurchase')}</Button>}>
    <DetailList columns={2} items={[
      { label: t('subscription.plan'), value: localized(plan.name, language) },
      { label: t('subscription.amount'), value: money(plan.price_cents, language) },
      { label: t('subscription.durationLabel'), value: t('subscription.duration', { count: plan.duration_days }) },
      { label: t('subscription.currentExpiry'), value: kind === 'renew' && <OrderTime value={current.expires_at} /> },
      { label: t('subscription.currentPlan'), value: kind === 'switch' && <>{current.plan && localized(current.plan.name, language)}<br /><OrderTime value={current.expires_at} /></> },
    ]} />
    {kind === 'switch' && <div className="mt-5 flex flex-col gap-4">
      <Notice tone="warning">{t('subscription.forfeit', { name: current.plan ? localized(current.plan.name, language) : t('subscription.noPlan'), expires: formatOrderTime(current.expires_at, language) })}</Notice>
      <Checkbox checked={confirmed} disabled={action.busy || stale} onChange={(event) => setConfirmed(event.target.checked)}>{t('subscription.switchConsent')}</Checkbox>
    </div>}
    {action.error != null && <div className="mt-5"><Notice tone="error">{t(subscriptionError(action.error), { defaultValue: t('polish.theOperationFailedPleaseTryAgain') })}{stale && <p className="mb-0 mt-2">{t('subscription.reopen')}</p>}</Notice></div>}
  </Dialog>
}

function PendingPurchase({ name, purchase, onChanged }: { name: string; purchase: components['schemas']['SubscriptionPurchaseDTO']; onChanged: () => Promise<void> }) {
  const { t, i18n } = useTranslation(), [cancel, setCancel] = useState(false)
  const detail = useQuery({ queryKey: ['merchant', name, 'subscription', 'purchase', purchase.id], queryFn: () => api.purchaseDetail(purchase.id), gcTime: 0 })
  const seconds = useCountdown(purchase.expires_at), value = detail.data ?? purchase
  return <Card aria-label={t('subscription.pending')}><SectionHeader title={t('subscription.pending')} description={t('subscription.timeLeft', { time: formatCountdown(seconds) })} />
    <DetailList columns={2} items={[{ label: t('subscription.plan'), value: localized(value.plan_snapshot.name, i18n.resolvedLanguage ?? 'zh-CN') }, { label: t('subscription.amount'), value: money(value.price_cents, i18n.resolvedLanguage ?? 'zh-CN') }, { label: t('subscription.created'), value: <OrderTime value={value.created_at} /> }, { label: t('subscription.tradeNo'), value: value.trade_no ? <CopyText value={value.trade_no} label={t('subscription.copyTradeNo')} /> : '—' }]} />
    {detail.isError && <ModuleError error={detail.error} retry={() => { void detail.refetch() }} />}
    <div className="mt-5 flex flex-wrap gap-3 border-t-[0.5px] border-border pt-5">
      {detail.data?.state === 'pending' && detail.data.cashier_url && seconds > 0 && <a className={buttonVariants({ variant: 'primary' })} href={detail.data.cashier_url}>{t('subscription.pay')}</a>}
      <Button variant="danger" onClick={() => setCancel(true)}>{t('subscription.cancel')}</Button>
    </div>
    {cancel && <CancelPurchaseDialog id={purchase.id} onClose={() => setCancel(false)} onChanged={onChanged} />}
  </Card>
}
export function CancelPurchaseDialog({ id, onClose, onChanged }: { id: string; onClose: () => void; onChanged: () => Promise<void> }) {
  const { t } = useTranslation(), action = useLocalAction()
  return <Dialog title={t('subscription.cancel')} description={t('subscription.cancelExplain')} onClose={onClose} busy={action.busy} footer={<><Button variant="secondary" disabled={action.busy} onClick={onClose}>{t('subscription.keepPurchase')}</Button><Button variant="dangerOutline" loading={action.busy} onClick={() => { void action.run(async () => {
    try { await api.cancelPurchase(id); await onChanged(); if (action.active.current) onClose() }
    catch (error) { if (error instanceof APIError && error.status === 409) await onChanged(); throw error }
  }) }}>{t('subscription.confirmCancel')}</Button></>}>
    {action.error != null && <Notice tone="error">{t(subscriptionError(action.error), { defaultValue: t('polish.theOperationFailedPleaseTryAgain') })}</Notice>}
  </Dialog>
}
