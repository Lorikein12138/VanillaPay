import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { X } from 'lucide-react'
import { Button } from './button'
import { Card, EmptyState } from './layout'
import { FormHint } from './auth-screen'
import { Notice } from './notice'
import { Table, Td, Th } from './table'
import { cn } from '@/lib/utils'

export function RecordLink({ to, children }: { to: string; children: ReactNode }) {
  return <Link to={to} className="text-accent-text underline decoration-accent-text/30 underline-offset-4 [overflow-wrap:anywhere] hover:decoration-current">{children}</Link>
}
export function RecordText({ children, secondary = false }: { children: ReactNode; secondary?: boolean }) {
  return <span className={secondary ? 'block break-words text-caption text-text-3 [overflow-wrap:anywhere]' : 'block min-w-0 break-words text-label [overflow-wrap:anywhere]'}>{children}</span>
}
export function RecordBadge({ children }: { children: ReactNode }) {
  return <span className="inline-flex rounded-full bg-muted px-2.5 py-0.5 text-caption text-text-2">{children}</span>
}
export function InlineActions({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap items-center gap-2">{children}</div>
}
export function FilterChip({ children, label, onClear }: { children: ReactNode; label: string; onClear: () => void }) {
  return <InlineActions><Button size="sm" variant="ghost" className="rounded-full bg-muted text-text hover:bg-secondary-hover"
    aria-label={label} onClick={onClear}>{children}<X aria-hidden="true" className="size-3.5" /></Button></InlineActions>
}
function filled(value: ReactNode) {
  return value !== null && value !== undefined && value !== false && value !== ''
}
/**
 * The same data becomes a table on desktop and compact cards on mobile: the
 * first column heads the card, the rest sit in two columns unless `wide`.
 */
export function Records<T>({ items, id, label, columns }: {
  items: T[]; id: (item: T) => string; label: string
  columns: { label: string; value: (item: T) => ReactNode; nowrap?: boolean; wide?: boolean }[]
}) {
  const [head, ...rest] = columns
  return <>
    <Table caption={label} className="hidden md:block" minWidth={720}>
      <thead><tr>{columns.map((column) => <Th key={column.label}>{column.label}</Th>)}</tr></thead>
      <tbody>{items.map((item) => <tr key={id(item)}>
        {columns.map((column) => <Td key={column.label} className={column.nowrap ? 'whitespace-nowrap' : undefined}>{column.value(item)}</Td>)}
      </tr>)}</tbody>
    </Table>
    <ul aria-label={label} className="m-0 flex list-none flex-col gap-4 p-0 md:hidden">
      {items.map((item) => <li key={id(item)}><Card>
        <dl className="m-0 grid grid-cols-2 gap-x-4 gap-y-3.5">
          {head && <div className="col-span-2 min-w-0 border-b-[0.5px] border-border pb-3.5">
            <dt className="sr-only">{head.label}</dt>
            <dd className="m-0 text-body font-medium [overflow-wrap:anywhere]">{head.value(item)}</dd>
          </div>}
          {rest.map((column) => ({ column, value: column.value(item) })).filter(({ value }) => filled(value)).map(({ column, value }) => (
            <div key={column.label} className={cn('min-w-0', column.wide && 'col-span-2')}>
              <dt className="mb-1 text-caption text-text-3">{column.label}</dt>
              <dd className="m-0 text-label text-text [overflow-wrap:anywhere]">{value}</dd>
            </div>
          ))}
        </dl>
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
