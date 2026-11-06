import { useRef, type ReactNode } from 'react'

/** Single selection with roving focus and automatic arrow-key selection. */
export function SegmentedControl<T extends string>({ label, value, options, onValueChange }: {
  label: string
  value: T
  options: { value: T; label: ReactNode; disabled?: boolean }[]
  onValueChange: (value: T) => void
}) {
  const group = useRef<HTMLDivElement>(null)
  const enabled = options.filter((option) => !option.disabled)
  const focusValue = enabled.some((option) => option.value === value) ? value : enabled[0]?.value
  return <div ref={group} role="radiogroup" aria-label={label} className="inline-flex w-fit max-w-full gap-1 rounded-control border-[0.5px] border-border bg-muted p-1">
    {options.map((option) => <button
      key={option.value}
      type="button"
      role="radio"
      aria-checked={value === option.value}
      tabIndex={focusValue === option.value ? 0 : -1}
      disabled={option.disabled}
      className="min-h-11 rounded-control px-4 text-label font-medium text-text-3 transition-colors hover:text-text aria-checked:bg-surface aria-checked:text-text disabled:opacity-45"
      onClick={() => onValueChange(option.value)}
      onKeyDown={(event) => {
        if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(event.key)) return
        event.preventDefault()
        const index = enabled.findIndex((entry) => entry.value === option.value)
        const nextIndex = event.key === 'Home' ? 0 : event.key === 'End' ? enabled.length - 1 : (index + (event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 1) + enabled.length) % enabled.length
        const next = enabled[nextIndex]
        if (!next) return
        onValueChange(next.value)
        group.current?.querySelectorAll<HTMLButtonElement>('[role="radio"]:not(:disabled)')[nextIndex]?.focus()
      }}
    >{option.label}</button>)}
  </div>
}
