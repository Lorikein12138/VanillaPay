import { Tooltip as RadixTooltip } from 'radix-ui'
import { useState, type ReactElement, type ReactNode } from 'react'

export const TooltipProvider = RadixTooltip.Provider

export function Tooltip({ label, children, enabled = true }: { label: ReactNode; children: ReactElement; enabled?: boolean }) {
  const [open, setOpen] = useState(false)
  return (
    <RadixTooltip.Root open={enabled && open} onOpenChange={setOpen}>
      <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
      <RadixTooltip.Portal>
        <RadixTooltip.Content side="right" sideOffset={10} collisionPadding={8}
          className="z-70 max-w-56 rounded-control border border-border-strong bg-surface px-3 py-2 text-caption break-words text-text shadow-lg">
          {label}
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    </RadixTooltip.Root>
  )
}
