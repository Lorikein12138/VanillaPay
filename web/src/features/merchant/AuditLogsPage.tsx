import { useInfiniteQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import { StatusDot } from '@/components/StatusDot'
import { Button } from '@/components/ui/button'
import { CopyField } from '@/components/ui/copy-button'
import { Dialog } from '@/components/ui/dialog'
import { DetailList } from '@/components/ui/detail-list'
import { Field, FieldGrid, FormActions } from '@/components/ui/field'
import { Input, Select } from '@/components/ui/input'
import { BackLink, Card, EmptyState, Mono, Page, PageHeader, Skeleton } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { Table, Td, Th } from '@/components/ui/table'
import { auditActions, type AuditFilters } from './audit-data'
import { ModuleError } from './CollectionParts'
import type { Schema } from './collection-data'
import { localDateInput } from './orders-data'
import { OrderTime } from './OrderParts'

export function AuditLogsPage({ name, filters, onFilters }: { name: string; filters: AuditFilters; onFilters: (value: AuditFilters) => void }) {
  const { t } = useTranslation(), [selected, setSelected] = useState<Schema['PortalAuditDTO'] | null>(null)
  const query = useInfiniteQuery({ queryKey: ['merchant', name, 'audit-logs', filters], queryFn: ({ pageParam }) => api.auditLogs({ ...filters, limit: 20, cursor: pageParam }), initialPageParam: undefined as string | undefined, getNextPageParam: (page) => page.next_cursor ?? undefined, gcTime: 0 })
  const items = query.data?.pages.flatMap((page) => page.items ?? []) ?? []
  const actionName = (action: string) => t(`audit.actions.${action}`, { defaultValue: action })
  const result = (value: string) => <StatusDot tone={value === 'success' ? 'ok' : 'off'}>{t(`audit.results.${value}`, { defaultValue: value })}</StatusDot>
  return <Page>
    <PageHeader title={t('audit.title')} description={t('audit.description')} back={<BackLink to="/account">{t('polish.accountSecurity')}</BackLink>} />
    <AuditFiltersForm key={JSON.stringify(filters)} filters={filters} onFilters={onFilters} />
    {query.isPending && <Skeleton className="h-64" />}
    {query.isError && <ModuleError error={query.error} retry={() => { void (query.isFetchNextPageError ? query.fetchNextPage() : query.refetch()) }} />}
    {query.data && (items.length ? <Table caption={t('audit.title')} minWidth={800}><thead><tr><Th>{t('audit.time')}</Th><Th>{t('audit.action')}</Th><Th>{t('audit.result')}</Th><Th>{t('audit.ip')}</Th><Th>{t('audit.request')}</Th><Th>{t('audit.details')}</Th></tr></thead><tbody>{items.map((item) => <tr key={item.id}>
      <Td><OrderTime value={item.created_at} /></Td><Td>{actionName(item.action)}</Td><Td>{result(item.result)}</Td><Td><Mono>{item.ip ?? '—'}</Mono></Td><Td className="max-w-64"><CopyField value={item.request_id} label={t('audit.copyRequest')} /></Td><Td><Button onClick={() => setSelected(item)}>{t('audit.details')}</Button></Td>
    </tr>)}</tbody></Table> : <EmptyState title={t('audit.empty')} />)}
    {query.hasNextPage && <div className="flex justify-center"><Button loading={query.isFetchingNextPage} onClick={() => { void query.fetchNextPage() }}>{t('polish.loadMore')}</Button></div>}
    {selected && <Dialog title={t('audit.details')} size="lg" onClose={() => setSelected(null)}>
      <DetailList columns={2} items={[{ label: t('audit.action'), value: actionName(selected.action) }, { label: t('audit.result'), value: result(selected.result) }, { label: t('audit.actor'), value: `${selected.actor_role} · ${selected.actor_id ?? '—'}`, mono: true }, { label: t('audit.target'), value: `${selected.target_type} · ${selected.target_id}`, mono: true }, { label: t('audit.time'), value: <OrderTime value={selected.created_at} /> }, { label: t('audit.ip'), value: selected.ip ?? '—', mono: true }]} />
      <div className="mt-5"><p className="text-caption text-text-3">{t('audit.request')}</p><CopyField value={selected.request_id} label={t('audit.copyRequest')} /></div>
      <div className="mt-5"><p className="text-caption text-text-3">{t('audit.metadata')}</p><pre className="m-0 whitespace-pre-wrap rounded-control bg-muted p-4"><Mono>{JSON.stringify(selected.safe_metadata, null, 2)}</Mono></pre></div>
    </Dialog>}
  </Page>
}
function AuditFiltersForm({ filters, onFilters }: { filters: AuditFilters; onFilters: (value: AuditFilters) => void }) {
  const { t } = useTranslation(), id = useId()
  const [action, setAction] = useState(filters.action ?? ''), [result, setResult] = useState(filters.result ?? '')
  const [start, setStart] = useState(localDateInput(filters.start)), [end, setEnd] = useState(localDateInput(filters.end)), [invalid, setInvalid] = useState(false)
  return <Card><form aria-label={t('audit.filters')} className="flex flex-col gap-5" onSubmit={(event) => {
    event.preventDefault()
    if ((start && Number.isNaN(Date.parse(start))) || (end && Number.isNaN(Date.parse(end))) || (start && end && Date.parse(start) >= Date.parse(end))) { setInvalid(true); return }
    setInvalid(false); onFilters({ action: action || undefined, result: result || undefined, start: start ? new Date(start).toISOString() : undefined, end: end ? new Date(end).toISOString() : undefined })
  }}><FieldGrid className="grid-cols-1 sm:grid-cols-2 xl:grid-cols-4">
    <Field label={t('audit.action')} htmlFor={id+'-action'}><Select id={id+'-action'} value={action} onChange={(event) => setAction(event.target.value)}><option value="">{t('audit.allActions')}</option>{action && !auditActions.some((value) => value === action) && <option value={action}>{action}</option>}{auditActions.map((value) => <option key={value} value={value}>{t(`audit.actions.${value}`, { defaultValue: value })}</option>)}</Select></Field>
    <Field label={t('audit.result')} htmlFor={id+'-result'}><Select id={id+'-result'} value={result} onChange={(event) => setResult(event.target.value)}><option value="">{t('audit.allResults')}</option><option value="success">{t('audit.results.success')}</option><option value="failure">{t('audit.results.failure')}</option></Select></Field>
    <Field label={t('polish.startTime')} htmlFor={id+'-start'}><Input id={id+'-start'} type="datetime-local" step="1" value={start} onChange={(event) => setStart(event.target.value)} /></Field>
    <Field label={t('polish.endTime')} htmlFor={id+'-end'}><Input id={id+'-end'} type="datetime-local" step="1" value={end} onChange={(event) => setEnd(event.target.value)} /></Field>
  </FieldGrid>{invalid && <Notice tone="error">{t('polish.enterValidDatesTheEndMustBe')}</Notice>}<FormActions className="border-t-[0.5px] border-border pt-5"><Button variant="ghost" onClick={() => { setAction(''); setResult(''); setStart(''); setEnd(''); setInvalid(false); onFilters({}) }}>{t('polish.reset')}</Button><Button type="submit" variant="primary">{t('polish.applyFilters')}</Button></FormActions></form></Card>
}
