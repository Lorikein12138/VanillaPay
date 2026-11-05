import { useQuery } from '@tanstack/react-query'
import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import { amountChange, money, previousRange, statsRange, successRate, type OverviewDays, type StatsRange } from './overview-data'
import { DeviceCard, RecentOrders, Skeleton, TrendChart, QueryProblem } from './OverviewCards'

function useStats(name: string, range: StatsRange) {
  return useQuery({ queryKey: ['merchant', name, 'stats', range], queryFn: () => api.stats(range), staleTime: 60000 })
}

export function Overview({ name }: { name: string }) {
  const { t, i18n } = useTranslation()
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  const [days, setDays] = useState<OverviewDays>(1)
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 30000)
    return () => window.clearInterval(timer)
  }, [])
  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const range = statsRange(now, days, timezone)
  const current = useStats(name, range)
  const previous = useStats(name, previousRange(range))
  const chart = useStats(name, statsRange(now, 7, timezone))
  const device = useQuery({ queryKey: ['merchant', name, 'device'], queryFn: api.device, refetchInterval: 30000 })
  const orders = useQuery({ queryKey: ['merchant', name, 'orders', 6], queryFn: () => api.orders(6) })
  const paid = current.data?.status_counts?.paid ?? 0
  const pending = current.data?.status_counts?.pending ?? 0
  const rate = successRate(paid, current.data?.status_counts?.expired ?? 0)
  const change = current.data && previous.data ? amountChange(current.data.total_amount_cents, previous.data.total_amount_cents) : null
  const decimal = new Intl.NumberFormat(language, { minimumFractionDigits: 1, maximumFractionDigits: 1 })
  const percent = new Intl.NumberFormat(language, { style: 'percent', minimumFractionDigits: 1, maximumFractionDigits: 1 })
  const period = t(`merchant.overview.range.${days}`)
  const greeting = now.getHours() < 12 ? 'morning' : now.getHours() < 18 ? 'afternoon' : 'evening'

  return (
    <div className="mx-auto flex max-w-[1112px] flex-col gap-12">
      <div className="flex flex-wrap items-end justify-between gap-5">
        <div className="min-w-0 space-y-1.5">
          <h1 className="m-0 break-words font-serif text-4xl font-[330] leading-tight">{t(`merchant.overview.greeting.${greeting}`, { name })}</h1>
          <p className="m-0 text-label text-text-2">{new Intl.DateTimeFormat(language, { dateStyle: 'full' }).format(now)}</p>
        </div>
        <div role="group" aria-label={t('merchant.overview.timeRange')} className="inline-flex rounded-[10px] border-[0.5px] border-border p-[3px]">
          {([1, 7, 30] as const).map((value) => <button key={value} type="button" aria-pressed={days === value} onClick={() => setDays(value)} className={`min-h-11 rounded-[7px] px-3.5 text-label transition-colors hover:bg-hover ${days === value ? 'bg-hover font-medium text-text' : 'text-text-2'}`}>{t(`merchant.overview.range.${value}`)}</button>)}
        </div>
      </div>

      <section aria-label={t('merchant.overview.metrics')} aria-busy={current.isPending} className="grid min-h-36 grid-cols-[repeat(auto-fit,minmax(min(200px,100%),1fr))] gap-8">
        {current.isError ? <div className="col-span-full"><QueryProblem retry={() => { void current.refetch() }} /></div> : <>
          <Metric label={t('merchant.overview.receipts', { period })} value={current.data ? money(current.data.total_amount_cents, language) : undefined}>
            {previous.isError ? <QueryProblem retry={() => { void previous.refetch() }} /> : previous.isPending ? <Skeleton className="h-4 w-36" /> : change === null ? t('merchant.overview.noPrevious') : t('merchant.overview.change', { value: new Intl.NumberFormat(language, { style: 'percent', signDisplay: 'always', minimumFractionDigits: 1, maximumFractionDigits: 1 }).format(change) })}
          </Metric>
          <Metric label={t('merchant.overview.orderCount', { period })} value={current.data?.total_order_count.toLocaleString(language)}>{t('merchant.overview.paidCount', { count: paid })}</Metric>
          <Metric label={t('merchant.overview.successRate')} value={current.data ? rate === null ? '—' : percent.format(rate) : undefined}>{t('merchant.overview.excludingPending', { count: pending })}</Metric>
          <Metric label={t('merchant.overview.pending')} accent value={current.data ? pending.toLocaleString(language) : undefined}>{t('merchant.overview.autoClose')}</Metric>
        </>}
      </section>

      <div className="flex flex-wrap gap-6">
        <section aria-labelledby="overview-trend" aria-busy={chart.isPending} className="min-h-[340px] min-w-0 flex-[2_1_440px] rounded-card border-[0.5px] border-border bg-surface p-5 sm:p-7">
          <div className="mb-6 flex flex-wrap items-baseline justify-between gap-2"><h2 id="overview-trend" className="font-serif text-heading">{t('merchant.overview.trend')}</h2><span className="text-caption text-text-3">{t('merchant.overview.thousands')}</span></div>
          {chart.isError ? <QueryProblem retry={() => { void chart.refetch() }} /> : chart.isPending ? <Skeleton className="h-[218px] w-full" /> : <TrendChart points={chart.data.trend ?? []} format={(cents) => decimal.format(cents / 100000)} />}
        </section>
        <section aria-labelledby="overview-device" aria-busy={device.isPending} className="min-h-[340px] min-w-0 flex-[1_1_280px] rounded-card border-[0.5px] border-border bg-surface p-5 sm:p-7">
          <h2 id="overview-device" className="mb-4 font-serif text-heading">{t('merchant.overview.device')}</h2>
          {device.isError ? <QueryProblem retry={() => { void device.refetch() }} /> : device.isPending ? <Skeleton className="h-20 w-full" /> : <DeviceCard device={device.data} now={now} />}
        </section>
      </div>

      <section aria-labelledby="overview-orders" aria-busy={orders.isPending} className="min-w-0">
        <h2 id="overview-orders" className="mb-4 font-serif text-heading">{t('merchant.overview.recentOrders')}</h2>
        {orders.isError ? <div className="min-h-80"><QueryProblem retry={() => { void orders.refetch() }} /></div> : orders.isPending ? <Skeleton className="h-80 w-full" /> : <RecentOrders orders={orders.data.items ?? []} />}
      </section>
    </div>
  )
}

function Metric({ label, value, children, accent = false }: { label: string; value?: string; children: ReactNode; accent?: boolean }) {
  return <section aria-label={label} className="min-w-0 border-t-[0.5px] border-border-strong pt-4"><h2 className="text-label text-text-2">{label}</h2>{value === undefined ? <Skeleton className="my-1.5 h-[46px] w-3/4" /> : <p className={`my-1.5 break-words font-serif text-figure tabular-nums ${accent ? 'text-accent-text' : ''}`}>{value}</p>}<div className="text-[13px] text-text-3">{value === undefined ? <Skeleton className="h-4 w-32" /> : children}</div></section>
}
