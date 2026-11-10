import { Check, ChevronDown, ChevronUp } from 'lucide-react'
import { Select as RadixSelect } from 'radix-ui'
import type { ComponentProps, ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { controlClass } from './input'

type SelectProps = Pick<ComponentProps<typeof RadixSelect.Trigger>, 'id' | 'aria-label' | 'aria-labelledby' | 'aria-describedby' | 'aria-invalid' | 'disabled'> & {
  value: string
  onValueChange: (value: string) => void
  children: ReactNode
}

const scrollClass = 'flex h-7 items-center justify-center text-text-3'

export function Select({ value, onValueChange, children, disabled, ...trigger }: SelectProps) {
  return (
    <RadixSelect.Root value={':' + value} onValueChange={(next) => onValueChange(next.slice(1))} disabled={disabled}>
      <RadixSelect.Trigger {...trigger} data-value={value} className={cn(controlClass, 'flex h-11 items-center justify-between gap-2 text-left [&>span:first-child]:truncate')}>
        <RadixSelect.Value />
        <RadixSelect.Icon asChild><ChevronDown aria-hidden="true" className="size-4 shrink-0 text-text-3" /></RadixSelect.Icon>
      </RadixSelect.Trigger>
      <RadixSelect.Portal>
        <RadixSelect.Content position="popper" align="start" sideOffset={4} collisionPadding={12}
          className="z-70 max-h-(--radix-select-content-available-height) min-w-(--radix-select-trigger-width) max-w-[calc(100dvw-24px)] overflow-hidden rounded-control border border-border-strong bg-surface text-text shadow-lg">
          <RadixSelect.ScrollUpButton className={scrollClass}><ChevronUp aria-hidden="true" className="size-4" /></RadixSelect.ScrollUpButton>
          <RadixSelect.Viewport className="p-1">{children}</RadixSelect.Viewport>
          <RadixSelect.ScrollDownButton className={scrollClass}><ChevronDown aria-hidden="true" className="size-4" /></RadixSelect.ScrollDownButton>
        </RadixSelect.Content>
      </RadixSelect.Portal>
    </RadixSelect.Root>
  )
}

export function SelectOption({ value, disabled, children }: { value: string; disabled?: boolean; children: ReactNode }) {
  return (
    <RadixSelect.Item value={':' + value} disabled={disabled} data-value={value}
      className="relative flex min-h-10 cursor-default items-center rounded-control py-2 pr-3 pl-9 text-label outline-none data-disabled:opacity-45 data-highlighted:bg-hover">
      <RadixSelect.ItemIndicator className="absolute left-3 text-accent-text"><Check aria-hidden="true" className="size-4" /></RadixSelect.ItemIndicator>
      <RadixSelect.ItemText>{children}</RadixSelect.ItemText>
    </RadixSelect.Item>
  )
}
