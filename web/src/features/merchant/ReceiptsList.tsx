import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import { Button } from '@/components/ui/button'
import { DetailList } from '@/components/ui/detail-list'
import { Dialog } from '@/components/ui/dialog'
import { Field, FieldGrid, FormActions } from '@/components/ui/field'
import { Input, Select } from '@/components/ui/input'
import { Card, EmptyState, Identifier, SectionHeader, Skeleton } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { Table, Th, Td } from '@/components/ui/table'
import { unmatchedReasons, type ReceiptFilters, type Schema } from './collection-data'
import { ModuleError } from './CollectionParts'
import { localDateInput, orderChannels } from './orders-data'
import { OrderChannel, OrderTime } from './OrderParts'
import { money } from './overview-data'

function ReceiptOrderLink({ tradeNo }: { tradeNo: string | null }) {
  if (!tradeNo) return null
  return (
    <Link to="/orders/$tradeNo" params={{ tradeNo }} search={{}} aria-label={tradeNo}
      className="inline-flex min-h-11 max-w-full items-center text-accent-text">
      <Identifier value={tradeNo} className="text-caption" />
    </Link>
  )
}

export function ReceiptsList({ name, filters, onFilters }: { name: string; filters: ReceiptFilters; onFilters: (value: ReceiptFilters) => void }) {
  const { t, i18n } = useTranslation()
  const [selected, setSelected] = useState<string | null>(null)
  const receipts = useInfiniteQuery({ queryKey: ['merchant', name, 'receipts', 'list', filters], gcTime: 0, initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => api.receipts({ ...filters, limit: 20, cursor: pageParam }), getNextPageParam: (page) => page.next_cursor ?? undefined })
  const items = receipts.data?.pages.flatMap((page) => page.items ?? []) ?? []
  const state = (item: Schema['PortalReceiptDTO']) => <>{item.match_state === 'matched' ? t('polish.matched') : t('polish.unmatched')}{item.unmatched_reason && <> · {t(unmatchedReasons[item.unmatched_reason] ?? 'polish.unknownReason')}</>}</>
  const idButton = (item: Schema['PortalReceiptDTO']) => <Button variant="link" className="max-w-full whitespace-normal text-left font-mono [overflow-wrap:anywhere]" onClick={() => setSelected(item.id)}>{item.id}</Button>
  const trade = (item: Schema['PortalReceiptDTO']) => <ReceiptOrderLink tradeNo={item.trade_no ?? null} />
  return <section aria-label={t('polish.receipts')} className="flex min-w-0 flex-col gap-5"><SectionHeader title={t('polish.receipts')} className="mb-0" />
    <ReceiptFilterForm key={JSON.stringify(filters)} filters={filters} onFilters={onFilters} />
    {receipts.isPending && <Skeleton className="h-64" />}
    {receipts.isError && <ModuleError error={receipts.error} retry={() => { void (receipts.isFetchNextPageError ? receipts.fetchNextPage() : receipts.refetch()) }} />}
    {receipts.data && <><p className="text-caption text-text-3">{t('polish.receiptTotal', { count: receipts.data.pages[0]?.total ?? 0 })}</p>
      {items.length ? <><Table caption={t('polish.receipts')} className="hidden md:block" minWidth={720}><thead><tr><Th>{t('polish.receiptId')}</Th><Th>{t('polish.paymentChannel')}</Th><Th>{t('polish.matchStatus')}</Th><Th align="right">{t('polish.paymentAmount')}</Th><Th>{t('polish.receivedAt')}</Th></tr></thead><tbody>{items.map((item) => <tr key={item.id}><Td className="max-w-64">{idButton(item)}{trade(item)}</Td><Td><OrderChannel channel={item.channel} /></Td><Td>{state(item)}</Td><Td align="right">{money(item.amount_cents, i18n.resolvedLanguage ?? 'zh-CN')}</Td><Td><OrderTime value={item.received_at} /></Td></tr>)}</tbody></Table>
        <ul className="m-0 flex list-none flex-col gap-3 p-0 md:hidden">{items.map((item) => <li key={item.id}><Card><div className="flex items-start justify-between gap-3"><div className="min-w-0">{idButton(item)}</div><span className="shrink-0 font-serif text-heading">{money(item.amount_cents, i18n.resolvedLanguage ?? 'zh-CN')}</span></div><p className="text-label text-text-2"><OrderChannel channel={item.channel} /> · {state(item)}</p>{trade(item)}<p className="mt-3 border-t-[0.5px] border-border pt-3 text-caption text-text-3">{t('polish.received')}<OrderTime value={item.received_at} /></p></Card></li>)}</ul></> : <EmptyState title={t('polish.noReceiptsMatchTheseFilters')} />}
      {receipts.hasNextPage && <div className="flex justify-center"><Button loading={receipts.isFetchingNextPage} onClick={() => { void receipts.fetchNextPage() }}>{receipts.isFetchingNextPage ? t('polish.loading2') : t('polish.loadMore')}</Button></div>}
    </>}
    {selected && <ReceiptDetail key={selected} name={name} id={selected} onClose={() => setSelected(null)} />}
  </section>
}
function ReceiptFilterForm({ filters, onFilters }: { filters: ReceiptFilters; onFilters: (value: ReceiptFilters) => void }) {
  const { t } = useTranslation()
  const id = useId(), [channel, setChannel] = useState(filters.channel ?? ''), [state, setState] = useState(filters.match_state ?? '')
  const [start, setStart] = useState(localDateInput(filters.start)), [end, setEnd] = useState(localDateInput(filters.end)), [invalid, setInvalid] = useState(false)
  return <Card><form aria-label={t('polish.receiptFilters')} className="flex flex-col gap-5" onSubmit={(event) => {
    event.preventDefault()
    if ((start && Number.isNaN(Date.parse(start))) || (end && Number.isNaN(Date.parse(end))) || (start && end && Date.parse(start) >= Date.parse(end))) { setInvalid(true); return }
    setInvalid(false); onFilters({ channel: channel || undefined, match_state: state || undefined, start: start ? new Date(start).toISOString() : undefined, end: end ? new Date(end).toISOString() : undefined })
  }}><FieldGrid className="grid-cols-1 sm:grid-cols-2 xl:grid-cols-4">
    <Field label={t('polish.paymentChannel')} htmlFor={`${id}-channel`}><Select id={`${id}-channel`} value={channel} onChange={(event) => setChannel(event.target.value)}><option value="">{t('polish.allChannels')}</option>{orderChannels.map((channel) => <option key={channel} value={channel}><OrderChannel channel={channel} /></option>)}</Select></Field>
    <Field label={t('polish.matchStatus')} htmlFor={`${id}-state`}><Select id={`${id}-state`} value={state} onChange={(event) => setState(event.target.value)}><option value="">{t('polish.allStates')}</option><option value="matched">{t('polish.matched')}</option><option value="unmatched">{t('polish.unmatched')}</option></Select></Field>
    <Field label={t('polish.startTime')} htmlFor={`${id}-start`}><Input id={`${id}-start`} type="datetime-local" step="1" value={start} onChange={(event) => setStart(event.target.value)} /></Field>
    <Field label={t('polish.endTime')} htmlFor={`${id}-end`}><Input id={`${id}-end`} type="datetime-local" step="1" value={end} onChange={(event) => setEnd(event.target.value)} /></Field>
  </FieldGrid>{invalid && <Notice tone="error">{t('polish.enterValidDatesTheEndMustBe')}</Notice>}<FormActions className="border-t-[0.5px] border-border pt-5"><Button variant="ghost" onClick={() => { setChannel(''); setState(''); setStart(''); setEnd(''); setInvalid(false); onFilters({}) }}>{t('polish.reset')}</Button><Button variant="primary" type="submit">{t('polish.applyFilters')}</Button></FormActions></form></Card>
}
function ReceiptDetail({ name, id, onClose }: { name: string; id: string; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const detail = useQuery({ queryKey: ['merchant', name, 'receipts', 'detail', id], queryFn: () => api.receipt(id), gcTime: 0 })
  const item = detail.data
  return <Dialog title={t('polish.receiptDetails')} description={t('polish.reviewTheOriginalReceiptAndOrderMatching')} size="lg" onClose={onClose}>
    {detail.isPending && <Skeleton className="h-64" />}{detail.isError && <ModuleError error={detail.error} retry={() => { void detail.refetch() }} />}
    {item && <DetailList columns={2} items={[
      { label: t('polish.receiptId'), value: item.id, mono: true }, { label: t('polish.eventId'), value: item.event_id, mono: true }, { label: t('polish.deviceId'), value: item.device_id, mono: true }, { label: t('polish.paymentChannel'), value: <OrderChannel channel={item.channel} /> }, { label: t('polish.paymentAmount'), value: money(item.amount_cents, i18n.resolvedLanguage ?? 'zh-CN') },
      { label: t('polish.occurredAt'), value: <OrderTime value={item.occurred_at} /> }, { label: t('polish.receivedAt'), value: <OrderTime value={item.received_at} /> }, { label: t('polish.providerTransactionId'), value: item.provider_trade_no ?? '—' }, { label: t('polish.payerHint'), value: item.payer_hint ?? '—' },
      { label: t('polish.matchStatus'), value: item.match_state === 'matched' ? t('polish.matched') : t('polish.unmatched') }, { label: t('polish.unmatchedReason'), value: item.unmatched_reason ? t(unmatchedReasons[item.unmatched_reason] ?? 'polish.unknownReason') : '—' },
      { label: t('polish.matchedOrder'), value: item.trade_no ? <ReceiptOrderLink tradeNo={item.trade_no} /> : null },
      { label: t('polish.rawText'), value: <pre className="m-0 whitespace-pre-wrap rounded-control bg-muted p-3 font-mono text-caption [overflow-wrap:anywhere]">{item.raw_text ?? '—'}</pre>, wide: true },
    ]} />}
  </Dialog>
}
