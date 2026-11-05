import { useInfiniteQuery } from '@tanstack/react-query'
import { Download } from 'lucide-react'
import { useId, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import { FormError } from '@/components/FormError'
import { Skeleton } from './OverviewCards'
import { localDateInput, orderChannels, orderErrorKey, orderStatuses, type OrderFilters } from './orders-data'
import { orderButton, orderInput, OrderRows } from './OrderParts'

export function OrdersPage({ name, filters, onFilters }: { name: string; filters: OrderFilters; onFilters: (value: OrderFilters) => void }) {
  const { t } = useTranslation()
  const [exporting, setExporting] = useState(false)
  const exportBusy = useRef(false)
  const [exportError, setExportError] = useState<unknown>()
  const orders = useInfiniteQuery({
    queryKey: ['merchant', name, 'orders', 'list', filters],
    // A different filter or a return from detail always starts at the first cursor.
    gcTime: 0,
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => api.orderPage({ ...filters, limit: 20, cursor: pageParam }),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
  })
  const items = orders.data?.pages.flatMap((page) => page.items ?? []) ?? []
  async function exportCSV() {
    if (exportBusy.current) return
    exportBusy.current = true
    setExporting(true)
    setExportError(undefined)
    try {
      const { blob, filename } = await api.exportOrders(filters)
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
  return <div className="mx-auto flex max-w-[1112px] flex-col gap-8">
    <div className="flex flex-wrap items-end justify-between gap-4"><div><h1 className="font-serif text-4xl font-[330]">{t('merchant.orders.title')}</h1><p className="mt-2 text-label text-text-2">{t('merchant.orders.description')}</p></div><button type="button" className={orderButton} disabled={exporting} onClick={() => { void exportCSV() }}><Download className="size-4" aria-hidden="true" />{t(exporting ? 'merchant.orders.exporting' : 'merchant.orders.export')}</button></div>
    {exportError != null && <FormError>{t(orderErrorKey(exportError))}</FormError>}
    <OrderFilterForm key={JSON.stringify(filters)} filters={filters} onApply={onFilters} />
    <section aria-label={t('merchant.orders.results')} aria-busy={orders.isPending} className="min-w-0">
      {orders.isPending ? <Skeleton className="h-80 w-full" /> : <>
        {orders.data && <p className="mb-4 text-caption text-text-3">{t('merchant.orders.total', { count: orders.data.pages[0]?.total ?? 0 })}</p>}
        {orders.isError && <div role="alert" className="mb-4 flex flex-wrap items-center gap-3 text-label"><span>{t(orderErrorKey(orders.error))}</span><button className={orderButton} onClick={() => { void (orders.isFetchNextPageError ? orders.fetchNextPage() : orders.refetch()) }}>{t('error.retry')}</button></div>}
        {orders.data && (items.length ? <OrderRows orders={items} filters={filters} /> : <p className="border-t-[0.5px] border-border py-16 text-center text-label text-text-3">{t('merchant.orders.empty')}</p>)}
        {orders.hasNextPage && <div className="mt-6 text-center"><button type="button" className={orderButton} disabled={orders.isFetchingNextPage} onClick={() => { void orders.fetchNextPage() }}>{t(orders.isFetchingNextPage ? 'merchant.orders.loading' : 'merchant.orders.more')}</button></div>}
      </>}
    </section>
  </div>
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
    if ((start && Number.isNaN(Date.parse(start))) || (end && Number.isNaN(Date.parse(end))) || (start && end && Date.parse(end) <= Date.parse(start)) || new TextEncoder().encode(query.trim()).length > 100) { setInvalid(true); return }
    setInvalid(false)
    onApply({ status: status || undefined, channel: channel || undefined, query: query.trim() || undefined, start: start ? new Date(start).toISOString() : undefined, end: end ? new Date(end).toISOString() : undefined })
  }
  return <form aria-label={t('merchant.orders.filters')} onSubmit={submit} className="rounded-card border-[0.5px] border-border bg-surface p-4 sm:p-5">
    <div className="grid grid-cols-[repeat(auto-fit,minmax(min(190px,100%),1fr))] items-end gap-4">
      <div className="min-w-0 space-y-2 text-caption text-text-2"><label htmlFor={`${id}-status`}>{t('merchant.overview.status')}</label><select id={`${id}-status`} className={orderInput} value={status} onChange={(event) => setStatus(event.target.value)}><option value="">{t('merchant.orders.allStatuses')}</option>{orderStatuses.map((value) => <option key={value} value={value}>{t(`merchant.overview.statuses.${value}`)}</option>)}</select></div>
      <div className="min-w-0 space-y-2 text-caption text-text-2"><label htmlFor={`${id}-channel`}>{t('merchant.overview.channel')}</label><select id={`${id}-channel`} className={orderInput} value={channel} onChange={(event) => setChannel(event.target.value)}><option value="">{t('merchant.orders.allChannels')}</option>{orderChannels.map((value) => <option key={value} value={value}>{t(`merchant.overview.channels.${value}`)}</option>)}</select></div>
      <div className="min-w-0 space-y-2 text-caption text-text-2"><label htmlFor={`${id}-start`}>{t('merchant.orders.start')}</label><input id={`${id}-start`} className={orderInput} type="datetime-local" step="1" value={start} onChange={(event) => setStart(event.target.value)} /></div>
      <div className="min-w-0 space-y-2 text-caption text-text-2"><label htmlFor={`${id}-end`}>{t('merchant.orders.end')}</label><input id={`${id}-end`} className={orderInput} type="datetime-local" step="1" value={end} onChange={(event) => setEnd(event.target.value)} /></div>
      <div className="min-w-0 space-y-2 text-caption text-text-2"><label htmlFor={`${id}-query`}>{t('merchant.orders.search')}</label><input id={`${id}-query`} className={orderInput} type="search" value={query} maxLength={100} placeholder={t('merchant.orders.searchPlaceholder')} onChange={(event) => setQuery(event.target.value)} /></div>
      <div className="flex gap-2"><button type="submit" className={orderButton}>{t('merchant.orders.apply')}</button><button type="button" className={orderButton} onClick={() => { setStatus(''); setChannel(''); setQuery(''); setStart(''); setEnd(''); setInvalid(false); onApply({}) }}>{t('merchant.orders.reset')}</button></div>
    </div>
    {invalid && <FormError>{t('merchant.orders.errors.filters')}</FormError>}
  </form>
}
