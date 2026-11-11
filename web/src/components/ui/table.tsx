import type { ReactNode, TdHTMLAttributes, ThHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

/**
 * Hairline table. Scrolls sideways inside its own box below `minWidth`, so the
 * page itself never overflows. Use a card list instead below `md` when rows
 * carry more than three columns.
 */
export function Table({
  caption,
  minWidth = 640,
  className,
  children,
}: {
  caption: string
  minWidth?: number
  className?: string
  children: ReactNode
}) {
  return (
    <div className={cn('min-w-0 overflow-x-auto', className)}>
      <table className="w-full border-collapse text-label" style={{ minWidth }}>
        <caption className="sr-only">{caption}</caption>
        {children}
      </table>
    </div>
  )
}

export function Th({ className, align, ...props }: ThHTMLAttributes<HTMLTableCellElement> & { align?: 'left' | 'right' }) {
  return (
    <th
      scope="col"
      className={cn(
        'whitespace-nowrap border-b-[0.5px] border-border-strong px-3 pb-3 pt-1 text-left text-caption font-[460] text-text-3 first:pl-0 last:pr-0',
        align === 'right' && 'text-right',
        className,
      )}
      {...props}
    />
  )
}

export function Td({ className, align, ...props }: TdHTMLAttributes<HTMLTableCellElement> & { align?: 'left' | 'right' }) {
  return (
    <td
      className={cn(
        'border-b-[0.5px] border-border px-3 py-4 align-middle first:pl-0 last:pr-0',
        align === 'right' && 'text-right tabular-nums',
        className,
      )}
      {...props}
    />
  )
}
