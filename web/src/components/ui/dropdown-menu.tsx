import { Check } from 'lucide-react'
import { DropdownMenu as RadixMenu } from 'radix-ui'
import { createContext, useContext, useRef, type ReactNode } from 'react'
import { focusQuietly } from '@/lib/quiet-focus'
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
  const triggerRef = useRef<HTMLButtonElement>(null)
  const pointerClose = useRef(false)
  return (
    <RadixMenu.Root modal={false}>
      <RadixMenu.Trigger asChild>
        <Button ref={triggerRef} size="icon" variant="ghost" disabled={disabled} aria-label={label} title={currentLabel} data-value={value}>
          {trigger}
        </Button>
      </RadixMenu.Trigger>
      <RadixMenu.Portal container={container ?? undefined}>
        <RadixMenu.Content align="end" sideOffset={4} collisionPadding={12}
          onPointerDown={() => { pointerClose.current = true }}
          onKeyDown={() => { pointerClose.current = false }}
          onCloseAutoFocus={(event) => {
            // A mouse pick returns focus without the keyboard ring; keyboard closes keep Radix's default.
            if (!pointerClose.current) return
            pointerClose.current = false
            event.preventDefault()
            focusQuietly(triggerRef.current)
          }}
          className="vp-pop z-70 min-w-40 rounded-xl bg-surface p-1 text-text shadow-pop">
          <RadixMenu.RadioGroup value={value} onValueChange={(next) => {
            const option = options.find((item) => item.value === next)
            if (option && option.value !== value) onValueChange(option.value)
          }}>
            {options.map((item) => (
              <RadixMenu.RadioItem key={item.value} value={item.value} lang={item.lang}
                className="flex min-h-10 cursor-default items-center gap-2.5 rounded-lg px-2.5 py-2 text-label outline-none data-highlighted:bg-hover">
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
