import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export type DetailItem = {
  label: ReactNode
  value: ReactNode
  /** Identifiers, URLs, keys: monospace and allowed to break anywhere. */
  mono?: boolean
  /** Span the full row (long URLs, free text). */
  wide?: boolean
}

function empty(value: ReactNode) {
  return value === null || value === undefined || value === false || value === ''
}

/**
 * Label/value grid with a hairline above each item. Items whose value is
 * empty are left out instead of showing a column of dashes.
 */
export function DetailList({ items, columns = 3, className }: { items: DetailItem[]; columns?: 2 | 3 | 4; className?: string }) {
  const shown = items.filter((item) => !empty(item.value))
  if (!shown.length) return null
  return (
    <dl className={cn('m-0 grid gap-x-8 sm:grid-cols-2', columns === 3 && 'lg:grid-cols-3', columns === 4 && 'lg:grid-cols-4', className)}>
      {shown.map((item, index) => (
        <div
          key={index}
          className={cn('min-w-0 border-t-[0.5px] border-border py-3.5', item.wide && 'sm:col-span-2 lg:col-span-full')}
        >
          <dt className="mb-1 text-caption text-text-3">{item.label}</dt>
          <dd
            className={cn(
              'm-0 whitespace-pre-wrap text-label text-text [overflow-wrap:anywhere]',
              item.mono && 'font-mono text-[13px]',
            )}
          >
            {item.value}
          </dd>
        </div>
      ))}
    </dl>
  )
}
