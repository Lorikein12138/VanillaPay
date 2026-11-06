import { Check, ChevronDown } from 'lucide-react'
import type { InputHTMLAttributes, ReactNode, SelectHTMLAttributes, TextareaHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

/** Base look for every text-like control: 44px, 1px strong border, accent focus ring. */
export const controlClass =
  'w-full min-w-0 rounded-control border border-border-strong bg-surface px-3 text-[15px] text-text transition-[border-color,box-shadow] duration-150 placeholder:text-text-3 focus:border-accent focus:shadow-[0_0_0_3px_var(--color-focus-ring)] focus:outline-none focus-visible:outline-none disabled:cursor-not-allowed disabled:bg-muted disabled:text-text-2 read-only:bg-muted/60 aria-invalid:border-danger'

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        controlClass,
        'h-11 [&::-webkit-calendar-picker-indicator]:cursor-pointer [&::-webkit-calendar-picker-indicator]:opacity-55 hover:[&::-webkit-calendar-picker-indicator]:opacity-90',
        className,
      )}
      {...props}
    />
  )
}

export function Textarea({ className, rows = 3, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea rows={rows} className={cn(controlClass, 'block min-h-11 resize-y py-2.5 leading-relaxed', className)} {...props} />
}

/** Native select (keeps keyboard, mobile pickers and tests) with the template's chevron. */
export function Select({ className, children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <span className="relative block min-w-0">
      <select className={cn(controlClass, 'h-11 cursor-pointer appearance-none pr-10', className)} {...props}>
        {children}
      </select>
      <ChevronDown
        aria-hidden="true"
        strokeWidth={1.8}
        className="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-text-3"
      />
    </span>
  )
}

/** Input with a trailing action in the same row, e.g. a code field with its send button. */
export function InputGroup({ children, action }: { children: ReactNode; action: ReactNode }) {
  return (
    <div className="flex min-w-0 gap-2">
      <div className="min-w-0 flex-1">{children}</div>
      {action}
    </div>
  )
}

/**
 * Native checkbox drawn to match the template. The label wraps the box so the
 * whole row is a 44px target.
 */
export function Checkbox({
  className,
  children,
  description,
  ...props
}: Omit<InputHTMLAttributes<HTMLInputElement>, 'type'> & { children: ReactNode; description?: ReactNode }) {
  return (
    <label
      className={cn(
        'group flex min-h-11 cursor-pointer items-start gap-3 py-2.5 text-label text-text has-[:disabled]:cursor-not-allowed has-[:disabled]:opacity-60',
        className,
      )}
    >
      <span className="relative mt-px grid size-[18px] shrink-0 place-items-center">
        <input
          type="checkbox"
          className="peer absolute inset-0 m-0 cursor-pointer appearance-none rounded-[5px] border border-border-strong bg-surface transition-colors checked:border-primary checked:bg-primary focus-visible:shadow-[0_0_0_3px_var(--color-focus-ring)] focus-visible:outline-1 focus-visible:outline-accent disabled:cursor-not-allowed"
          {...props}
        />
        <Check
          aria-hidden="true"
          strokeWidth={2.6}
          className="pointer-events-none relative size-3 text-primary-fg opacity-0 transition-opacity peer-checked:opacity-100"
        />
      </span>
      <span className="min-w-0">
        <span className="block leading-[1.4]">{children}</span>
        {description && <span className="mt-1 block text-caption text-text-3">{description}</span>}
      </span>
    </label>
  )
}
