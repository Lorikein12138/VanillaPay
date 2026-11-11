import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

/** Compact rows inside one Card, with the same hairline as shared tables. */
export function List({ children, label }: { children: ReactNode; label: string }) {
  return <ul aria-label={label} className="m-0 list-none divide-y-[0.5px] divide-border p-0">{children}</ul>
}

export function ListRow({ title, label, description, meta, actions, unread = false }: {
  title: ReactNode
  label: string
  description: ReactNode
  meta: ReactNode
  actions?: ReactNode
  unread?: boolean
}) {
  return <li aria-label={label} className="min-w-0 py-6 first:pt-0 last:pb-0">
    <div className="flex items-center gap-2">
      {unread && <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-accent" />}
      <h3 className={cn('m-0 min-w-0 break-words font-sans text-label', unread ? 'font-semibold text-text' : 'font-medium text-text-2')}>{title}</h3>
    </div>
    <p className={cn('mb-2 mt-2 whitespace-pre-wrap break-words text-label leading-relaxed', unread ? 'text-text-2' : 'text-text-3')}>{description}</p>
    <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1">
      <div className="text-caption text-text-3">{meta}</div>
      {actions && <div className="flex items-center gap-3">{actions}</div>}
    </div>
  </li>
}
