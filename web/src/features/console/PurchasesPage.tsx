import { plansQuery, purchasesQuery } from '@/app/portal-queries'
import { useResetPortalCursors } from '@/app/portal-cursors'
import { DateTimeField } from '@/components/ui/date-time-picker'
import { localDateInput, parseLocalDateTime } from '@/lib/date-time'
import { SelectOption } from '@/components/ui/select'
import { useState } from 'react'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { StatusDot } from '@/components/StatusDot'
import { Button } from '@/components/ui/button'
import { FormStack } from '@/components/ui/auth-screen'
import { FieldGrid, FormActions } from '@/components/ui/field'
import { Card, Page, PageHeader } from '@/components/ui/layout'
import { FilterChip, RecordLink, RecordResults, Records } from '@/components/ui/records'
import { AccountError } from '@/features/auth/account-shared'
import { OrderTime } from '@/features/merchant/OrderParts'
import { money } from '@/features/merchant/overview-data'
import { Choice } from './ManageParts'
import { planName, purchaseStates, type PurchaseFilters } from './manage-data'

export function PurchasesPage({ filters, onFilters }: { filters: PurchaseFilters; onFilters: (filters: PurchaseFilters) => void }) {
  const { t, i18n } = useTranslation()
  const options = purchasesQuery(filters)
  const query = useInfiniteQuery(options)
  useResetPortalCursors(options.queryKey)
  const items = query.data?.pages.flatMap((page) => page.items ?? []) ?? []
  return <Page>
    <PageHeader title={t('manage.purchases')} />
    {filters.merchant_id && <FilterChip onClear={() => onFilters({ ...filters, merchant_id: undefined })} label={t('manage.clearMerchant')}>
      {t('manage.merchantFilter', { id: filters.merchant_id })}
    </FilterChip>}
    <PurchaseFiltersForm key={JSON.stringify(filters)} filters={filters} onFilters={onFilters} />
    <RecordResults busy={query.isPending} error={query.error} retry={() => void query.refetch()} total={query.data?.pages[0]?.total}
      empty={!!query.data && !items.length} more={query.hasNextPage && !query.isPlaceholderData} loadingMore={query.isFetchingNextPage} onMore={() => void query.fetchNextPage()}>
      <Records items={items} id={(item) => item.id} label={t('manage.purchases')} columns={[
        { label: t('manage.created'), nowrap: true, value: (item) => <OrderTime value={item.created_at} /> },
        { label: t('manage.merchant'), nowrap: true, value: (item) => <RecordLink to={'/console/merchants/' + item.buyer_id}>#{item.buyer_id}</RecordLink> },
        { label: t('manage.plan'), value: (item) => planName(item.plan_snapshot, i18n.language) },
        { label: t('manage.kind'), nowrap: true, value: (item) => t('manage.kinds.' + item.purchase_kind, { defaultValue: item.purchase_kind }) },
        { label: t('merchant.overview.amount'), nowrap: true, value: (item) => money(item.price_cents, i18n.language) },
        { label: t('manage.status'), nowrap: true, value: (item) => <StatusDot tone={item.state === 'paid' ? 'ok' : item.state === 'pending' ? 'wait' : 'off'}>
          {t('manage.states.' + item.state, { defaultValue: item.state })}
        </StatusDot> },
        { label: t('manage.paymentOrder'), wide: true, value: (item) => item.trade_no && <span className="font-mono text-caption"><RecordLink to={'/console/orders/' + item.trade_no}>{item.trade_no}</RecordLink></span> },
        { label: t('manage.paidExpiry'), nowrap: true, wide: true, value: (item) => <span className="flex flex-col gap-0.5">
          <span>{t('manage.paidShort')} <OrderTime value={item.paid_at} /></span>
          <span className="text-text-3">{t('manage.expiresShort')} <OrderTime value={item.expires_at} /></span>
        </span> },
      ]} />
    </RecordResults>
  </Page>
}
function PurchaseFiltersForm({ filters, onFilters }: { filters: PurchaseFilters; onFilters: (value: PurchaseFilters) => void }) {
  const { t, i18n } = useTranslation(), [state, setState] = useState(filters.state ?? ''), [planId, setPlanId] = useState(String(filters.plan_id ?? ''))
  const [start, setStart] = useState(localDateInput(filters.start)), [end, setEnd] = useState(localDateInput(filters.end))
  const plans = useQuery(plansQuery(true))
  const valid = (!start || Number.isFinite(parseLocalDateTime(start))) && (!end || Number.isFinite(parseLocalDateTime(end))) && (!start || !end || parseLocalDateTime(start) < parseLocalDateTime(end))
  return <Card><FormStack onSubmit={(event) => {
    event.preventDefault()
    if (valid) onFilters({ merchant_id: filters.merchant_id, state: state || undefined, plan_id: planId ? Number(planId) : undefined,
      start: start ? new Date(start).toISOString() : undefined, end: end ? new Date(end).toISOString() : undefined })
  }}>
    {plans.error != null && <AccountError error={plans.error} />}
    <FieldGrid>
    <Choice label={t('manage.status')} value={state} onChange={setState}><SelectOption value="">{t('manage.all')}</SelectOption>
      {purchaseStates.map((item) => <SelectOption key={item} value={item}>{t(`manage.states.${item}`)}</SelectOption>)}
    </Choice>
    <Choice label={t('manage.plan')} value={planId} onChange={setPlanId}><SelectOption value="">{t('manage.all')}</SelectOption>
      {plans.data?.items?.map((item) => <SelectOption key={item.id} value={item.id}>{planName(item, i18n.language)}{item.deleted ? ' · ' + t('manage.deleted') : ''}</SelectOption>)}
    </Choice>
    <DateTimeField label={t('merchant.orders.start')} value={start} onValueChange={setStart} />
    <DateTimeField label={t('merchant.orders.end')} value={end} onValueChange={setEnd} />
  </FieldGrid><FormActions>
    <Button onClick={() => { setState(''); setPlanId(''); setStart(''); setEnd(''); onFilters({}) }}>{t('manage.resetFilters')}</Button>
    <Button type="submit" variant="primary" disabled={!valid}>{t('manage.apply')}</Button>
  </FormActions></FormStack></Card>
}
