import { plansQuery } from '@/app/portal-queries'
import { useState } from 'react'
import { keepPreviousData, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi } from '@/api/client'
import { FormHint, FormStack } from '@/components/ui/auth-screen'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { DetailList } from '@/components/ui/detail-list'
import { FormActions, FieldGrid } from '@/components/ui/field'
import { Checkbox } from '@/components/ui/input'
import { Page, PageHeader } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { InlineActions, RecordResults, Records } from '@/components/ui/records'
import { AccountError, AccountField } from '@/features/auth/account-shared'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { centsToYuan, yuanToCents } from '@/features/merchant/collection-data'
import { money } from '@/features/merchant/overview-data'
import { ChannelChoices, Channels } from './ManageParts'
import { channels, invalidateConsole, planInput, planName, validPurchaseLimit, type Schema } from './manage-data'

type Plan = Schema['SubscriptionPlanDTO']
const amountFields = ['min_order_cents', 'max_order_cents', 'daily_limit_cents'] as const
export function PlansPage() {
  const { t, i18n } = useTranslation(), [deleted, setDeleted] = useState(false), [notice, setNotice] = useState('')
  const [dialog, setDialog] = useState<{ mode: 'edit' | 'delete'; plan?: Plan } | null>(null)
  const query = useQuery({ ...plansQuery(deleted), placeholderData: keepPreviousData })
  return <Page>
    <PageHeader title={t('manage.plans')} actions={<Button variant="primary" onClick={() => setDialog({ mode: 'edit' })}>{t('manage.createPlan')}</Button>} />
    <Checkbox checked={deleted} onChange={(event) => setDeleted(event.target.checked)}>{t('manage.showDeleted')}</Checkbox>
    {notice && <Notice tone="success" onDismiss={() => setNotice('')}>{notice}</Notice>}
    <RecordResults busy={query.isPending} error={query.error} retry={() => void query.refetch()} empty={!!query.data && !query.data.items?.length}>
      <Records items={query.data?.items ?? []} id={(item) => item.id} label={t('manage.plans')} columns={[
        { label: t('manage.plan'), value: (item) => <>{item.name.zh_CN}<FormHint>{item.name.en}</FormHint>{item.deleted && <FormHint>{t('manage.deleted')}</FormHint>}</> },
        { label: t('manage.price'), value: (item) => money(item.price_cents, i18n.language) },
        { label: t('manage.days'), value: (item) => item.duration_days },
        { label: t('manage.channels'), value: (item) => <Channels value={item.channels} />, wide: true },
        { label: t('manage.limit'), value: (item) => item.purchase_limit ?? t('manage.unlimited') },
        { label: t('manage.purchased'), value: (item) => item.purchased_count ?? '—' },
        { label: t('manage.remaining'), value: (item) => item.remaining_count ?? '—' },
        { label: t('manage.manage'), wide: true, value: (item) => item.deleted ? t('manage.deleted') : <InlineActions>
          <Button size="sm" onClick={() => setDialog({ mode: 'edit', plan: item })}>{t('manage.edit')}</Button>
          <Button size="sm" variant="dangerOutline" onClick={() => setDialog({ mode: 'delete', plan: item })}>{t('manage.remove')}</Button>
        </InlineActions> },
      ]} />
    </RecordResults>
    {dialog && <PlanDialog plan={dialog.plan} deleting={dialog.mode === 'delete'} onClose={() => setDialog(null)}
      onSuccess={(message) => { setNotice(message); setDialog(null) }} />}
  </Page>
}
function PlanDialog({ plan, deleting, onClose, onSuccess }: {
  plan?: Plan; deleting: boolean; onClose: () => void; onSuccess: (message: string) => void
}) {
  const { t, i18n } = useTranslation(), action = useLocalAction(), client = useQueryClient()
  const [zh, setZh] = useState(plan?.name.zh_CN ?? ''), [en, setEn] = useState(plan?.name.en ?? '')
  const [price, setPrice] = useState(plan ? centsToYuan(plan.price_cents) : '')
  const [days, setDays] = useState(plan ? String(plan.duration_days) : '')
  const [limit, setLimit] = useState(plan?.purchase_limit == null ? '' : String(plan.purchase_limit))
  const [amountLimits, setAmountLimits] = useState(() => Object.fromEntries(amountFields.map((field) => {
    const cents = plan?.[field]
    return [field, cents == null ? '' : centsToYuan(cents)]
  })))
  const [selected, setSelected] = useState(plan?.channels ?? []), [revision, setRevision] = useState(plan?.revision)
  const [latest, setLatest] = useState<Plan | null>(null), [conflict, setConflict] = useState(false), [reviewed, setReviewed] = useState(false)
  const value = planInput(zh, en, price, days, selected, limit)
  const [minimum, maximum, daily] = amountFields.map((field) => yuanToCents(amountLimits[field] ?? ''))
  const invalidAmount = (field: typeof amountFields[number], cents: number | null | undefined) =>
    !!amountLimits[field] && (cents == null || cents < 1 || cents > 999999999)
  const orderInvalid = minimum != null && maximum != null && minimum > maximum
  const valid = (deleting || (!!value && !orderInvalid && !amountFields.some((field, index) => invalidAmount(field, [minimum, maximum, daily][index])))) && (!conflict || reviewed) && !latest?.deleted
  const title = t(deleting ? 'manage.deletePlan' : plan ? 'manage.editPlan' : 'manage.createPlan')
  const submitLabel = plan && !deleting ? t('manage.savePlan') : title
  async function submit() {
    if (!valid) return
    await action.run(async () => {
      try {
        let message = t('manage.planSaved')
        if (deleting && plan && revision !== undefined) {
          const result = await consoleApi.deletePlan(plan.id, revision)
          message = t('manage.deletedResult', { count: result.cancelled_pending_purchases })
        } else if (value) await consoleApi.savePlan({
          ...value, ...(plan ? { id: plan.id, revision } : {}),
          min_order_cents: minimum ?? null, max_order_cents: maximum ?? null, daily_limit_cents: daily ?? null,
        })
        await invalidateConsole(client)
        if (!action.active.current) return
        if (action.active.current) onSuccess(message)
      } catch (error) {
        if (!action.active.current) return
        if (error instanceof APIError && error.status === 409 && plan) {
          const refreshed = await consoleApi.plans(true)
          if (!action.active.current) return
          const current = refreshed.items?.find((item) => item.id === plan.id)
          setConflict(true); setReviewed(false); setLatest(current ?? { ...plan, deleted: true }); setRevision(current?.revision)
          await invalidateConsole(client)
        } else throw error
      }
    })
  }
  return <Dialog title={title} description={deleting ? t('manage.deleteDescription') : undefined} onClose={onClose} busy={action.busy}>
    <FormStack onSubmit={(event) => { event.preventDefault(); void submit() }}>
      {conflict && <>
        <Notice tone="warning">{t('manage.planConflict')}</Notice>
        {latest && <DetailList columns={2} items={[
          { label: t('manage.zhName'), value: latest.name.zh_CN },
          { label: t('manage.enName'), value: latest.name.en || '—' },
          { label: t('manage.channels'), value: <Channels value={latest.channels} /> },
          { label: t('manage.limit'), value: latest.purchase_limit ?? t('manage.unlimited') },
          { label: t('manage.price'), value: money(latest.price_cents, i18n.language) },
          ...amountFields.map((field) => {
            const cents = latest[field]
            return { label: t(`risk.plan.${field}`), value: cents == null ? t('manage.unlimited') : money(cents, i18n.language) }
          }),
          { label: t('manage.days'), value: latest.duration_days },
          { label: t('manage.status'), value: latest.deleted ? t('manage.deleted') : t('manage.active') },
        ]} />}
        <Checkbox checked={reviewed} disabled={action.busy || !!latest?.deleted} onChange={(event) => setReviewed(event.target.checked)}>{t('manage.reviewed')}</Checkbox>
      </>}
      {!deleting && <>
        <FieldGrid>
          <AccountField label={t('manage.zhName')} hint={t('manage.nameHint')} value={zh} disabled={action.busy} onChange={(event) => setZh(event.target.value)} />
          <AccountField label={t('manage.enName')} hint={t('manage.enNameHint')} value={en} disabled={action.busy} onChange={(event) => setEn(event.target.value)} />
          <AccountField label={t('manage.price')} hint={t('manage.priceHint')} inputMode="decimal" value={price} disabled={action.busy} onChange={(event) => setPrice(event.target.value)} />
          <AccountField label={t('manage.days')} hint={t('manage.daysHint')} inputMode="numeric" value={days} disabled={action.busy} onChange={(event) => setDays(event.target.value)} />
          <AccountField label={t('manage.limit')} hint={t('manage.limitHint')} inputMode="numeric" value={limit} disabled={action.busy}
            error={!validPurchaseLimit(limit) && t('manage.limitInvalid')} onChange={(event) => setLimit(event.target.value)} />
          {amountFields.map((field, index) => <AccountField key={field} label={t(`risk.plan.${field}`)} hint={t('risk.plan.hint')} inputMode="decimal" value={amountLimits[field] ?? ''} disabled={action.busy}
            error={invalidAmount(field, [minimum, maximum, daily][index]) ? t('risk.plan.invalid') : field === 'max_order_cents' && orderInvalid ? t('risk.plan.range') : undefined}
            onChange={(event) => setAmountLimits((previous) => ({ ...previous, [field]: event.target.value }))} />)}
        </FieldGrid>
        <ChannelChoices allowed={channels} value={selected} onChange={setSelected} disabled={action.busy} />
      </>}
      {deleting && plan && <FormHint>{planName(plan, i18n.language)}</FormHint>}
      {action.error != null && <AccountError error={action.error} />}
      <FormActions><Button onClick={onClose} disabled={action.busy}>{t('manage.cancel')}</Button>
        <Button type="submit" loading={action.busy} disabled={!valid} variant={deleting ? 'dangerOutline' : 'primary'}>{submitLabel}</Button></FormActions>
    </FormStack>
  </Dialog>
}
