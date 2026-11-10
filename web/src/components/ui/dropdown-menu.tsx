import { Check } from 'lucide-react'
import { DropdownMenu as RadixMenu } from 'radix-ui'
import { createContext, useContext, type ReactNode } from 'react'
import { Button } from './button'

export const DropdownMenuContainer = createContext<HTMLElement | null>(null)

export function DropdownMenu<T extends string>({ label, currentLabel, trigger, disabled, value, options, onValueChange }: {
  label: string
  currentLabel: string
  trigger: ReactNode
  disabled?: boolean
  value: T
  options: readonly { value: T; label: string; icon?: ReactNode; lang?: string }[]
  onValueChange: (value: T) => void
}) {
  const container = useContext(DropdownMenuContainer)
  return (
    <RadixMenu.Root modal={false}>
      <RadixMenu.Trigger asChild>
        <Button size="icon" variant="ghost" disabled={disabled} aria-label={label} title={currentLabel} data-value={value}>
          {trigger}
        </Button>
      </RadixMenu.Trigger>
      <RadixMenu.Portal container={container ?? undefined}>
        <RadixMenu.Content align="end" sideOffset={4} collisionPadding={12}
          className="z-70 min-w-36 rounded-control border border-border-strong bg-surface p-1 text-text shadow-lg">
          <RadixMenu.RadioGroup value={value} onValueChange={(next) => {
            const option = options.find((item) => item.value === next)
            if (option && option.value !== value) onValueChange(option.value)
          }}>
            {options.map((item) => (
              <RadixMenu.RadioItem key={item.value} value={item.value} lang={item.lang}
                className="flex min-h-10 cursor-default items-center gap-2 rounded-control px-2.5 py-2 text-label outline-none data-highlighted:bg-hover">
                {item.icon}
                <span className="flex-1">{item.label}</span>
                <span className="grid size-4 place-items-center text-accent-text">
                  <RadixMenu.ItemIndicator><Check aria-hidden="true" className="size-4" /></RadixMenu.ItemIndicator>
                </span>
              </RadixMenu.RadioItem>
            ))}
          </RadixMenu.RadioGroup>
        </RadixMenu.Content>
      </RadixMenu.Portal>
    </RadixMenu.Root>
  )
}
