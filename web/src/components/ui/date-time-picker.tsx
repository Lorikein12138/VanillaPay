import { CalendarDays, ChevronLeft, ChevronRight, X } from 'lucide-react'
import { Popover } from 'radix-ui'
import { useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { calendarDateValue, parseCalendarDate, parseLocalDateTime } from '@/lib/date-time'
import { cn } from '@/lib/utils'
import { Button } from './button'
import { Field } from './field'
import { Input } from './input'

export function DateTimePicker({ id, value, onValueChange, disabled, invalid, describedBy }: {
  id: string
  value: string
  onValueChange: (value: string) => void
  disabled?: boolean
  invalid?: boolean
  describedBy?: string
}) {
  const { t } = useTranslation()
  const input = useRef<HTMLInputElement>(null)
  return (
    <Popover.Root>
      <div className="relative min-w-0">
        <Input ref={input} id={id} value={value} disabled={disabled} aria-invalid={invalid || undefined}
          aria-describedby={describedBy} data-empty={!value} autoComplete="off" spellCheck={false}
          placeholder="YYYY-MM-DDTHH:mm:ss" className={value && !disabled ? 'pr-[76px]' : 'pr-10'}
          onChange={(event) => onValueChange(event.target.value)} />
        <div className="absolute inset-y-0 right-0 flex items-center">
          {value && !disabled && (
            <Button size="icon" variant="ghost" className="size-9" aria-label={t('ui.clearDateTime')} onClick={() => {
              onValueChange('')
              input.current?.focus()
            }}><X aria-hidden="true" /></Button>
          )}
          <Popover.Trigger asChild>
            <Button size="icon" variant="ghost" className="size-10" disabled={disabled}
              aria-label={t('ui.openDateTime')}><CalendarDays aria-hidden="true" /></Button>
          </Popover.Trigger>
        </div>
      </div>
      <Popover.Portal>
        <Popover.Content align="start" sideOffset={4} collisionPadding={12} aria-label={t('ui.openDateTime')}
          className="z-70 max-h-(--radix-popover-content-available-height) w-[calc(100dvw-24px)] max-w-[22rem] overflow-y-auto rounded-dialog border border-border-strong bg-surface p-3 text-text shadow-lg">
          <Calendar value={value} onApply={onValueChange} />
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  )
}

function Calendar({ value, onApply }: { value: string; onApply: (value: string) => void }) {
  const { t, i18n } = useTranslation()
  const timeId = useId()
  const selected = parseCalendarDate(value.slice(0, 10))
  const [active, setActive] = useState(() => selected ?? new Date())
  const [time, setTime] = useState(() => Number.isFinite(parseLocalDateTime(value)) ? value.split('T')[1]! : '00:00:00')
  const year = active.getFullYear(), month = active.getMonth()
  const first = new Date(active)
  first.setDate(1)
  const monthDays = new Date(first)
  monthDays.setMonth(month + 1, 0)
  const dateFormat = new Intl.DateTimeFormat(i18n.language, { dateStyle: 'full' })
  const monthLabel = new Intl.DateTimeFormat(i18n.language, { year: 'numeric', month: 'long' }).format(first)
  const weekdayFormat = new Intl.DateTimeFormat(i18n.language, { weekday: 'short' })
  const activeDay = useRef<HTMLButtonElement>(null), focusDay = useRef(false)
  const nextValue = `${calendarDateValue(active)}T${time}`
  const validTime = Number.isFinite(parseLocalDateTime(nextValue))

  useLayoutEffect(() => {
    if (focusDay.current) {
      activeDay.current?.focus()
      focusDay.current = false
    }
  }, [active])

  function moveMonth(offset: number) {
    const next = new Date(first)
    next.setMonth(month + offset)
    if (next.getFullYear() >= 1 && next.getFullYear() <= 9999) setActive(next)
  }

  function moveDay(event: KeyboardEvent<HTMLButtonElement>) {
    const offsets: Record<string, number> = {
      ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7,
      Home: -active.getDay(), End: 6 - active.getDay(),
    }
    const offset = offsets[event.key]
    if (offset === undefined) return
    event.preventDefault()
    const next = new Date(active)
    next.setDate(active.getDate() + offset)
    if (next.getFullYear() < 1 || next.getFullYear() > 9999) return
    focusDay.current = true
    setActive(next)
  }

  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between">
        <Button variant="ghost" size="icon" disabled={year <= 1 && month === 0}
          aria-label={t('ui.previousMonth')} onClick={() => moveMonth(-1)}><ChevronLeft aria-hidden="true" /></Button>
        <span aria-live="polite" className="text-label font-medium">{monthLabel}</span>
        <Button variant="ghost" size="icon" disabled={year >= 9999 && month === 11}
          aria-label={t('ui.nextMonth')} onClick={() => moveMonth(1)}><ChevronRight aria-hidden="true" /></Button>
      </div>
      <div role="group" aria-label={monthLabel} className="grid grid-cols-7 text-center">
        {Array.from({ length: 7 }, (_, index) => {
          const label = weekdayFormat.format(new Date(2026, 0, 4 + index))
          return <span key={index} aria-hidden="true" className="py-1 text-caption text-text-3">{label}</span>
        })}
        {Array.from({ length: first.getDay() }, (_, index) => <span key={`blank-${index}`} aria-hidden="true" />)}
        {Array.from({ length: monthDays.getDate() }, (_, index) => {
          const date = new Date(first)
          date.setDate(index + 1)
          const focused = active.getDate() === date.getDate()
          return <Button key={index} variant="ghost" size="icon" ref={focused ? activeDay : undefined}
            tabIndex={focused ? 0 : -1} onKeyDown={moveDay} aria-label={dateFormat.format(date)}
            aria-pressed={focused} className={cn('h-9 w-full', focused && 'bg-muted text-text')}
            onClick={() => setActive(date)}>{index + 1}</Button>
        })}
      </div>
      <Field label={t('ui.time')} htmlFor={timeId}>
        <Input id={timeId} value={time} placeholder="HH:mm:ss" aria-invalid={!validTime || undefined}
          onChange={(event) => setTime(event.target.value)} />
      </Field>
      <Popover.Close asChild>
        <Button variant="primary" disabled={!validTime} onClick={() => onApply(nextValue)}>{t('ui.applyDateTime')}</Button>
      </Popover.Close>
    </div>
  )
}

export function DateTimeField({ label, error, ...props }: Omit<Parameters<typeof DateTimePicker>[0], 'id'> & {
  label: string
  error?: string | false
}) {
  const id = useId()
  return (
    <Field label={label} htmlFor={id} error={error}>
      <DateTimePicker {...props} id={id} invalid={props.invalid || !!error} describedBy={error ? id + '-hint' : props.describedBy} />
    </Field>
  )
}
