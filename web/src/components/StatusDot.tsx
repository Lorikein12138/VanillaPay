import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

const tones = { ok: 'bg-dot-ok', wait: 'bg-dot-wait', info: 'bg-dot-info', off: 'bg-dot-off' } as const

export function StatusDot({ tone, children }: { tone: keyof typeof tones; children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-2 text-label text-text-2">
      <span aria-hidden="true" className={cn('size-[7px] shrink-0 rounded-full', tones[tone])} />
      {children}
    </span>
  )
}
