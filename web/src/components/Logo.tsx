import { cn } from '@/lib/utils'

export function LogoMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 28 28" aria-hidden="true" className={cn('size-7 shrink-0', className)}>
      <rect width="28" height="28" rx="7" fill="#D97757" />
      <path
        d="M8.5 9l5.5 11 5.5-11"
        fill="none"
        stroke="#FAF9F5"
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
