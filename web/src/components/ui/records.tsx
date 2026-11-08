import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { Button } from './button'
import { Card, EmptyState } from './layout'
import { FormHint } from './auth-screen'
import { Notice } from './notice'
import { DetailList, type DetailItem } from './detail-list'
import { Table, Td, Th } from './table'

export function RecordLink({ to, children }: { to: string; children: ReactNode }) {
  return <Link to={to} className="text-accent-text underline decoration-border-strong underline-offset-4 [overflow-wrap:anywhere] hover:decoration-current">{children}</Link>
}
export function RecordText({ children, secondary = false }: { children: ReactNode; secondary?: boolean }) {
  return <span className={secondary ? 'block break-words text-caption text-text-3 [overflow-wrap:anywhere]' : 'block min-w-0 break-words text-label [overflow-wrap:anywhere]'}>{children}</span>
}
export function RecordBadge({ children }: { children: ReactNode }) {
  return <span className="inline-flex rounded-full border-[0.5px] border-border bg-muted px-2.5 py-1 text-caption text-text-2">{children}</span>
}
export function InlineActions({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-2">{children}</div>
}
export function FilterChip({ children, label, onClear }: { children: ReactNode; label: string; onClear: () => void }) {
  return <InlineActions><Button size="sm" variant="ghost" className="rounded-full border-[0.5px] border-border bg-muted"
    aria-label={label} onClick={onClear}>{children}<X aria-hidden="true" className="size-3.5" /></Button></InlineActions>
}
/** The same data becomes a table on desktop and labelled cards on mobile. */
export function Records<T>({ items, id, label, columns }: {
  items: T[]; id: (item: T) => string; label: string
  columns: { label: string; value: (item: T) => ReactNode; nowrap?: boolean }[]
}) {
  return <>
    <Table caption={label} className="hidden md:block" minWidth={720}>
      <thead><tr>{columns.map((column) => <Th key={column.label}>{column.label}</Th>)}</tr></thead>
      <tbody>{items.map((item) => <tr key={id(item)}>
        {columns.map((column) => <Td key={column.label} className={column.nowrap ? 'whitespace-nowrap' : undefined}>{column.value(item)}</Td>)}
      </tr>)}</tbody>
    </Table>
    <ul aria-label={label} className="m-0 flex list-none flex-col gap-4 p-0 md:hidden">
      {items.map((item) => <li key={id(item)}><Card>
        <DetailList columns={2} items={columns.map((column): DetailItem => ({ label: column.label, value: column.value(item) }))} />
      </Card></li>)}
    </ul>
  </>
}
export function RecordResults({ busy, error, retry, total, more, loadingMore, onMore, children, empty }: {
  busy: boolean; error: unknown; retry: () => void; total?: number; more?: boolean
  loadingMore?: boolean; onMore?: () => void; children: ReactNode; empty?: boolean
}) {
  const { t } = useTranslation()
  return <section aria-busy={busy} className="flex min-w-0 flex-col gap-4">
    {busy && <FormHint>{t('auth.loading')}</FormHint>}
    {error != null && <Notice tone="error" action={<Button onClick={retry}>{t('error.retry')}</Button>}>{t('manage.loadFailed')}</Notice>}
    {total !== undefined && <FormHint>{t('manage.total', { count: total })}</FormHint>}
    {empty ? <EmptyState title={t('manage.empty')} /> : children}
    {more && <InlineActions><Button loading={loadingMore} onClick={onMore}>{t('merchant.orders.more')}</Button></InlineActions>}
  </section>
}
