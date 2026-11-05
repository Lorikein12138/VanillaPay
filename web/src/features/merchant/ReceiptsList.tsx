import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useId, useState } from 'react'
import { api } from '@/api/client'
import { FormError } from '@/components/FormError'
import { unmatchedReasons, type ReceiptFilters } from './collection-data'
import { ModuleDialog, ModuleError, modulePanel } from './CollectionParts'
import { localDateInput, orderChannels } from './orders-data'
import { OrderChannel, OrderTime, orderButton, orderInput } from './OrderParts'
import { money } from './overview-data'

export function ReceiptsList({ name, filters, onFilters }: { name: string; filters: ReceiptFilters; onFilters: (value: ReceiptFilters) => void }) {
  const [selected, setSelected] = useState<string | null>(null)
  const receipts = useInfiniteQuery({ queryKey: ['merchant', name, 'receipts', 'list', filters], gcTime: 0, initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => api.receipts({ ...filters, limit: 20, cursor: pageParam }), getNextPageParam: (page) => page.next_cursor ?? undefined })
  const items = receipts.data?.pages.flatMap((page) => page.items ?? []) ?? []
  return <section aria-label="收款记录" className="min-w-0 space-y-5"><h2 className="font-serif text-2xl">收款记录</h2>
    <ReceiptFilterForm key={JSON.stringify(filters)} filters={filters} onFilters={onFilters} />
    {receipts.isPending && <p className="text-label text-text-2">正在加载收款记录…</p>}
    {receipts.isError && <ModuleError error={receipts.error} retry={() => { void (receipts.isFetchNextPageError ? receipts.fetchNextPage() : receipts.refetch()) }} />}
    {receipts.data && <><p className="text-caption text-text-3">共 {receipts.data.pages[0]?.total ?? 0} 条收款记录 · 时间筛选依据为接收时间</p>
      {items.length ? <ul className="space-y-3">{items.map((item) => <li key={item.id} className={`${modulePanel} flex flex-wrap items-center justify-between gap-4`}>
        <div className="min-w-0 space-y-2"><button className="break-all text-left font-mono text-label text-accent-text underline-offset-4 hover:underline" onClick={() => setSelected(item.id)}>{item.id}</button><p className="text-label text-text-2"><OrderChannel channel={item.channel} /> · {item.match_state === 'matched' ? '已匹配' : '未匹配'}{item.unmatched_reason && ` · ${unmatchedReasons[item.unmatched_reason] ?? '未知原因'}`}</p><p className="text-caption text-text-3">接收：<OrderTime value={item.received_at} /></p></div>
        <div className="space-y-2"><p className="font-serif text-xl">{money(item.amount_cents, 'zh-CN')}</p>{item.trade_no && <Link to="/orders/$tradeNo" params={{ tradeNo: item.trade_no }} search={{}} className="break-all font-mono text-caption text-accent-text">{item.trade_no}</Link>}</div>
      </li>)}</ul> : <p className="py-12 text-center text-label text-text-3">没有符合筛选条件的收款记录</p>}
      {receipts.hasNextPage && <button className={orderButton} disabled={receipts.isFetchingNextPage} onClick={() => { void receipts.fetchNextPage() }}>{receipts.isFetchingNextPage ? '加载中…' : '加载更多'}</button>}
    </>}
    {selected && <ReceiptDetail key={selected} name={name} id={selected} onClose={() => setSelected(null)} />}
  </section>
}
function ReceiptFilterForm({ filters, onFilters }: { filters: ReceiptFilters; onFilters: (value: ReceiptFilters) => void }) {
  const id = useId(), [channel, setChannel] = useState(filters.channel ?? ''), [state, setState] = useState(filters.match_state ?? '')
  const [start, setStart] = useState(localDateInput(filters.start)), [end, setEnd] = useState(localDateInput(filters.end)), [invalid, setInvalid] = useState(false)
  return <form aria-label="收款记录筛选" className={modulePanel} onSubmit={(event) => {
    event.preventDefault()
    if ((start && Number.isNaN(Date.parse(start))) || (end && Number.isNaN(Date.parse(end))) || (start && end && Date.parse(start) >= Date.parse(end))) { setInvalid(true); return }
    setInvalid(false); onFilters({ channel: channel || undefined, match_state: state || undefined, start: start ? new Date(start).toISOString() : undefined, end: end ? new Date(end).toISOString() : undefined })
  }}><div className="grid items-end gap-4 sm:grid-cols-2 lg:grid-cols-4">
    <label className="space-y-2 text-label" htmlFor={`${id}-channel`}><span>收款渠道</span><select id={`${id}-channel`} className={orderInput} value={channel} onChange={(event) => setChannel(event.target.value)}><option value="">全部渠道</option>{orderChannels.map((channel) => <option key={channel} value={channel}>{channel === 'alipay' ? '支付宝' : channel === 'wxpay' ? '微信支付' : 'QQ 钱包'}</option>)}</select></label>
    <label className="space-y-2 text-label" htmlFor={`${id}-state`}><span>匹配状态</span><select id={`${id}-state`} className={orderInput} value={state} onChange={(event) => setState(event.target.value)}><option value="">全部状态</option><option value="matched">已匹配</option><option value="unmatched">未匹配</option></select></label>
    <label className="space-y-2 text-label" htmlFor={`${id}-start`}><span>开始时间</span><input id={`${id}-start`} type="datetime-local" step="1" className={orderInput} value={start} onChange={(event) => setStart(event.target.value)} /></label>
    <label className="space-y-2 text-label" htmlFor={`${id}-end`}><span>结束时间</span><input id={`${id}-end`} type="datetime-local" step="1" className={orderInput} value={end} onChange={(event) => setEnd(event.target.value)} /></label>
  </div><div className="mt-4 flex gap-3"><button className={orderButton} type="submit">应用筛选</button><button className={orderButton} type="button" onClick={() => { setChannel(''); setState(''); setStart(''); setEnd(''); setInvalid(false); onFilters({}) }}>重置</button></div>{invalid && <FormError>请填写有效时间，结束时间须晚于开始时间。</FormError>}</form>
}
function ReceiptDetail({ name, id, onClose }: { name: string; id: string; onClose: () => void }) {
  const detail = useQuery({ queryKey: ['merchant', name, 'receipts', 'detail', id], queryFn: () => api.receipt(id), gcTime: 0 })
  const item = detail.data
  return <ModuleDialog title="收款记录详情" description="查看原始上报内容及订单匹配结果。" onClose={onClose}>
    {detail.isPending && <p>正在加载…</p>}{detail.isError && <ModuleError error={detail.error} retry={() => { void detail.refetch() }} />}
    {item && <dl className="space-y-4 break-all text-label">{[
      ['记录 ID', item.id], ['事件 ID', item.event_id], ['设备 ID', item.device_id], ['收款渠道', <OrderChannel channel={item.channel} />], ['收款金额', money(item.amount_cents, 'zh-CN')],
      ['发生时间', <OrderTime value={item.occurred_at} />], ['接收时间', <OrderTime value={item.received_at} />], ['渠道流水号', item.provider_trade_no ?? '—'], ['付款方提示', item.payer_hint ?? '—'],
      ['匹配状态', item.match_state === 'matched' ? '已匹配' : '未匹配'], ['未匹配原因', item.unmatched_reason ? unmatchedReasons[item.unmatched_reason] ?? '未知原因' : '—'],
    ].map(([label, value], index) => <div key={index}><dt className="mb-1 text-text-3">{label}</dt><dd>{value}</dd></div>)}
      {item.trade_no && <div><dt className="mb-1 text-text-3">匹配订单</dt><dd><Link to="/orders/$tradeNo" params={{ tradeNo: item.trade_no }} search={{}} className="text-accent-text">{item.trade_no}</Link></dd></div>}
      <div><dt className="mb-1 text-text-3">原始文本</dt><dd><pre className="whitespace-pre-wrap break-all rounded-control bg-muted p-3 font-mono text-caption">{item.raw_text ?? '—'}</pre></dd></div>
    </dl>}
  </ModuleDialog>
}
