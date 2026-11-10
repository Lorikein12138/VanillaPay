import { ordersQuery } from '@/app/portal-queries'
import { useResetPortalCursors } from '@/app/portal-cursors'
import { DateTimePicker } from '@/components/ui/date-time-picker'
import { localDateInput, parseLocalDateTime } from '@/lib/date-time'
import { Select, SelectOption } from '@/components/ui/select'
import { hashKey, useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { Download, Inbox } from 'lucide-react'
import { useId, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { api, consoleApi } from '@/api/client'
import { FormError } from '@/components/FormError'
import { Button } from '@/components/ui/button'
import { Field, FieldGrid, FormActions } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Card, EmptyState, Page, PageHeader, Skeleton } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { orderChannels, orderErrorKey, orderStatuses, type OrderFilters } from './orders-data'
import { OrderRows } from './OrderParts'

export function OrdersPage({ name, filters, onFilters, role = 'merchant' }: { role?: 'merchant' | 'console'; name: string; filters: OrderFilters; onFilters: (value: OrderFilters) => void }) {
  const { t } = useTranslation()
  const [exporting, setExporting] = useState(false)
  const exportBusy = useRef(false)
  const [exportError, setExportError] = useState<unknown>()
  const client = useQueryClient()
  const options = ordersQuery(role, name, filters)
  const orders = useInfiniteQuery(options)
  useResetPortalCursors(options.queryKey)
  const items = orders.data?.pages.flatMap((page) => page.items ?? []) ?? []
  function applyFilters(next: OrderFilters) {
    if (hashKey([next]) === hashKey([filters])) {
      client.setQueryData(options.queryKey, (data) => data && { pages: data.pages.slice(0, 1), pageParams: data.pageParams.slice(0, 1) })
      void client.invalidateQueries({ queryKey: options.queryKey, exact: true })
    }
    onFilters(next)
  }
  async function exportCSV() {
    if (exportBusy.current) return
    exportBusy.current = true
    setExporting(true)
    setExportError(undefined)
    try {
      const { blob, filename } = await (role === 'console' ? consoleApi : api).exportOrders(filters)
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = filename
      document.body.append(link)
      link.click()
      link.remove()
      window.setTimeout(() => URL.revokeObjectURL(url), 1000)
    } catch (error) { setExportError(error) }
    finally { exportBusy.current = false; setExporting(false) }
  }
  return (
    <Page className="gap-8">
      <PageHeader
        title={t('merchant.orders.title')}
        description={t('merchant.orders.description')}
        actions={
          <Button loading={exporting} onClick={() => { void exportCSV() }}>
            {!exporting && <Download aria-hidden="true" strokeWidth={1.8} />}
            {t(exporting ? 'merchant.orders.exporting' : 'merchant.orders.export')}
          </Button>
        }
      />
      {exportError != null && (
        <Notice tone="error" onDismiss={() => setExportError(undefined)}>{t(orderErrorKey(exportError))}</Notice>
      )}
      <OrderFilterForm key={JSON.stringify(filters)} filters={filters} onApply={applyFilters} />
      <section aria-label={t('merchant.orders.results')} aria-busy={orders.isPending} className="flex min-w-0 flex-col gap-4">
        {orders.isPending ? (
          <div role="status" aria-label={t('polish.loading')} className="flex flex-col gap-3">
            <Skeleton className="h-4 w-28" />
            <Skeleton className="h-80 w-full" />
          </div>
        ) : (
          <>
            {orders.data && (
              <p className="m-0 text-[13px] text-text-3">{t('merchant.orders.total', { count: orders.data.pages[0]?.total ?? 0 })}</p>
            )}
            {orders.isError && (
              <Notice
                tone="error"
                action={
                  <Button size="sm" onClick={() => { void (orders.isFetchNextPageError ? orders.fetchNextPage() : orders.refetch()) }}>
                    {t('error.retry')}
                  </Button>
                }
              >
                {t(orderErrorKey(orders.error))}
              </Notice>
            )}
            {orders.data && (items.length ? (
              <OrderRows orders={items} filters={filters} role={role} />
            ) : (
              <EmptyState icon={<Inbox strokeWidth={1.5} />} title={t('merchant.orders.empty')} />
            ))}
            {orders.hasNextPage && !orders.isPlaceholderData && (
              <div className="flex justify-center pt-2">
                <Button loading={orders.isFetchingNextPage} onClick={() => { void orders.fetchNextPage() }}>
                  {t(orders.isFetchingNextPage ? 'merchant.orders.loading' : 'merchant.orders.more')}
                </Button>
              </div>
            )}
          </>
        )}
      </section>
    </Page>
  )
}

function OrderFilterForm({ filters, onApply }: { filters: OrderFilters; onApply: (value: OrderFilters) => void }) {
  const { t } = useTranslation()
  const id = useId()
  const [status, setStatus] = useState(filters.status ?? '')
  const [channel, setChannel] = useState(filters.channel ?? '')
  const [query, setQuery] = useState(filters.query ?? '')
  const [start, setStart] = useState(localDateInput(filters.start))
  const [end, setEnd] = useState(localDateInput(filters.end))
  const [invalid, setInvalid] = useState(false)
  function submit(event: FormEvent) {
    event.preventDefault()
    if ((start && Number.isNaN(parseLocalDateTime(start))) || (end && Number.isNaN(parseLocalDateTime(end))) || (start && end && parseLocalDateTime(end) <= parseLocalDateTime(start)) || new TextEncoder().encode(query.trim()).length > 100) { setInvalid(true); return }
    setInvalid(false)
    onApply({ status: status || undefined, channel: channel || undefined, query: query.trim() || undefined, start: start ? new Date(start).toISOString() : undefined, end: end ? new Date(end).toISOString() : undefined })
  }
  function reset() {
    setStatus(''); setChannel(''); setQuery(''); setStart(''); setEnd(''); setInvalid(false)
    onApply({})
  }
  return (
    <Card className="p-5 sm:p-6">
      <form aria-label={t('merchant.orders.filters')} onSubmit={submit} className="flex flex-col gap-5">
        {/* Explicit columns so every row fills: search spans two, the other four pair up. */}
        <FieldGrid className="grid-cols-1 sm:grid-cols-2 xl:grid-cols-3">
          <Field label={t('merchant.orders.search')} htmlFor={`${id}-query`} className="sm:col-span-2">
            <Input id={`${id}-query`} type="search" value={query} maxLength={100} placeholder={t('merchant.orders.searchPlaceholder')} onChange={(event) => setQuery(event.target.value)} />
          </Field>
          <Field label={t('merchant.overview.status')} htmlFor={`${id}-status`}>
            <Select id={`${id}-status`} value={status} onValueChange={setStatus}>
              <SelectOption value="">{t('merchant.orders.allStatuses')}</SelectOption>
              {orderStatuses.map((value) => <SelectOption key={value} value={value}>{t(`merchant.overview.statuses.${value}`)}</SelectOption>)}
            </Select>
          </Field>
          <Field label={t('merchant.overview.channel')} htmlFor={`${id}-channel`}>
            <Select id={`${id}-channel`} value={channel} onValueChange={setChannel}>
              <SelectOption value="">{t('merchant.orders.allChannels')}</SelectOption>
              {orderChannels.map((value) => <SelectOption key={value} value={value}>{t(`merchant.overview.channels.${value}`)}</SelectOption>)}
            </Select>
          </Field>
          <Field label={t('merchant.orders.start')} htmlFor={`${id}-start`}>
            <DateTimePicker id={`${id}-start`} value={start} invalid={invalid} onValueChange={setStart} />
          </Field>
          <Field label={t('merchant.orders.end')} htmlFor={`${id}-end`}>
            <DateTimePicker id={`${id}-end`} value={end} invalid={invalid} onValueChange={setEnd} />
          </Field>
        </FieldGrid>
        {invalid && <FormError>{t('merchant.orders.errors.filters')}</FormError>}
        <FormActions className="border-t-[0.5px] border-border pt-5">
          <Button variant="ghost" onClick={reset}>{t('merchant.orders.reset')}</Button>
          <Button type="submit" variant="primary">{t('merchant.orders.apply')}</Button>
        </FormActions>
      </form>
    </Card>
  )
}
