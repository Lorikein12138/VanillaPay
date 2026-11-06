import type { ReactNode } from 'react'
import { FormError } from '@/components/FormError'
import { cn } from '@/lib/utils'

/**
 * Label + control + hint/error stack. The caller gives the control `id={htmlFor}`
 * and, when a hint or error is shown, `aria-describedby={`${htmlFor}-hint`}`.
 */
export function Field({
  label,
  htmlFor,
  hint,
  error,
  optional,
  className,
  children,
}: {
  label: ReactNode
  htmlFor: string
  hint?: ReactNode
  error?: ReactNode
  optional?: ReactNode
  className?: string
  children: ReactNode
}) {
  return (
    <div className={cn('flex min-w-0 flex-col gap-1.5', className)}>
      <label htmlFor={htmlFor} className="flex items-baseline justify-between gap-3 text-label text-text-2">
        <span>{label}</span>
        {optional && <span className="text-caption text-text-3">{optional}</span>}
      </label>
      {children}
      {error ? (
        <FormError id={`${htmlFor}-hint`}>{error}</FormError>
      ) : hint ? (
        <p id={`${htmlFor}-hint`} className="m-0 text-caption leading-relaxed text-text-3">
          {hint}
        </p>
      ) : null}
    </div>
  )
}

/** Responsive grid for filter bars and short forms; fields wrap at ~190px. */
export function FieldGrid({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn('grid grid-cols-[repeat(auto-fit,minmax(min(190px,100%),1fr))] items-start gap-4', className)}>{children}</div>
}

/** Right-aligned action row at the end of a form; stacks full width on phones. */
export function FormActions({ className, children }: { className?: string; children: ReactNode }) {
  return <div className={cn('flex flex-col-reverse gap-2 sm:flex-row sm:flex-wrap sm:items-center sm:justify-end', className)}>{children}</div>
}
