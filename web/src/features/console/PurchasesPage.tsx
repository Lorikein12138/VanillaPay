import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { consoleApi } from '@/api/client'
import { StatusDot } from '@/components/StatusDot'
import { Button } from '@/components/ui/button'
import { FormStack } from '@/components/ui/auth-screen'
import { FieldGrid, FormActions } from '@/components/ui/field'
import { Card, Page, PageHeader } from '@/components/ui/layout'
import { FilterChip, RecordLink, RecordResults, Records } from '@/components/ui/records'
import { AccountError } from '@/features/auth/account-shared'
import { AccountField } from '@/features/auth/account-shared'
import { OrderTime } from '@/features/merchant/OrderParts'
import { localDateInput } from '@/features/merchant/orders-data'
import { money } from '@/features/merchant/overview-data'
import { Choice } from './ManageParts'
import { planName, purchaseStates, useCursorList, type PurchaseFilters } from './manage-data'

export function PurchasesPage({ filters, onFilters }: { filters: PurchaseFilters; onFilters: (filters: PurchaseFilters) => void }) {
  const { t, i18n } = useTranslation()
  const query = useCursorList(['console', 'purchases', filters], (cursor, signal) => consoleApi.purchases({ ...filters, cursor, limit: 20 }, signal))
  const items = query.data?.pages.flatMap((page) => page.items ?? []) ?? []
  return <Page>
    <PageHeader title={t('manage.purchases')} />
    {filters.merchant_id && <FilterChip onClear={() => onFilters({ ...filters, merchant_id: undefined })} label={t('manage.clearMerchant')}>
      {t('manage.merchantFilter', { id: filters.merchant_id })}
    </FilterChip>}
    <PurchaseFiltersForm key={JSON.stringify(filters)} filters={filters} onFilters={onFilters} />
    <RecordResults busy={query.isPending} error={query.error} retry={() => void query.refetch()} total={query.data?.pages[0]?.total}
      empty={!!query.data && !items.length} more={query.hasNextPage} loadingMore={query.isFetchingNextPage} onMore={() => void query.fetchNextPage()}>
      <Records items={items} id={(item) => item.id} label={t('manage.purchases')} columns={[
        { label: t('manage.created'), value: (item) => <OrderTime value={item.created_at} /> },
        { label: t('manage.merchant'), value: (item) => <RecordLink to={'/console/merchants/' + item.buyer_id}>#{item.buyer_id}</RecordLink> },
        { label: t('manage.plan'), value: (item) => planName(item.plan_snapshot, i18n.language) },
        { label: t('manage.kind'), value: (item) => t('manage.kinds.' + item.purchase_kind, { defaultValue: item.purchase_kind }) },
        { label: t('merchant.overview.amount'), value: (item) => money(item.price_cents, i18n.language) },
        { label: t('manage.status'), value: (item) => <StatusDot tone={item.state === 'paid' ? 'ok' : item.state === 'pending' ? 'wait' : 'off'}>
          {t('manage.states.' + item.state, { defaultValue: item.state })}
        </StatusDot> },
        { label: t('manage.paymentOrder'), value: (item) => item.trade_no && <RecordLink to={'/console/orders/' + item.trade_no}>{item.trade_no}</RecordLink> },
        { label: t('manage.paidExpiry'), value: (item) => <><OrderTime value={item.paid_at} /> / <OrderTime value={item.expires_at} /></> },
      ]} />
    </RecordResults>
  </Page>
}
function PurchaseFiltersForm({ filters, onFilters }: { filters: PurchaseFilters; onFilters: (value: PurchaseFilters) => void }) {
  const { t, i18n } = useTranslation(), [state, setState] = useState(filters.state ?? ''), [planId, setPlanId] = useState(String(filters.plan_id ?? ''))
  const [start, setStart] = useState(localDateInput(filters.start)), [end, setEnd] = useState(localDateInput(filters.end))
  const plans = useQuery({ queryKey: ['console', 'plans', true], queryFn: ({ signal }) => consoleApi.plans(true, signal) })
  const valid = (!start || Number.isFinite(Date.parse(start))) && (!end || Number.isFinite(Date.parse(end))) && (!start || !end || Date.parse(start) < Date.parse(end))
  return <Card><FormStack onSubmit={(event) => {
    event.preventDefault()
    if (valid) onFilters({ merchant_id: filters.merchant_id, state: state || undefined, plan_id: planId ? Number(planId) : undefined,
      start: start ? new Date(start).toISOString() : undefined, end: end ? new Date(end).toISOString() : undefined })
  }}>
    {plans.error != null && <AccountError error={plans.error} />}
    <FieldGrid>
    <Choice label={t('manage.status')} value={state} onChange={setState}><option value="">{t('manage.all')}</option>
      {purchaseStates.map((item) => <option key={item} value={item}>{t(`manage.states.${item}`)}</option>)}
    </Choice>
    <Choice label={t('manage.plan')} value={planId} onChange={setPlanId}><option value="">{t('manage.all')}</option>
      {plans.data?.items?.map((item) => <option key={item.id} value={item.id}>{planName(item, i18n.language)}{item.deleted ? ' · ' + t('manage.deleted') : ''}</option>)}
    </Choice>
    <AccountField label={t('merchant.orders.start')} type="datetime-local" step="1" value={start} onChange={(event) => setStart(event.target.value)} />
    <AccountField label={t('merchant.orders.end')} type="datetime-local" step="1" value={end} onChange={(event) => setEnd(event.target.value)} />
  </FieldGrid><FormActions>
    <Button onClick={() => { setState(''); setPlanId(''); setStart(''); setEnd(''); onFilters({}) }}>{t('manage.resetFilters')}</Button>
    <Button type="submit" variant="primary" disabled={!valid}>{t('manage.apply')}</Button>
  </FormActions></FormStack></Card>
}
