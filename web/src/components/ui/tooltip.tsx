import { Tooltip as RadixTooltip } from 'radix-ui'
import { useState, type ReactElement, type ReactNode } from 'react'

export const TooltipProvider = RadixTooltip.Provider

export function Tooltip({ label, children, enabled = true }: { label: ReactNode; children: ReactElement; enabled?: boolean }) {
  const [state, setState] = useState({ enabled, open: false })
  if (state.enabled !== enabled) setState({ enabled, open: false })
  return (
    <RadixTooltip.Root open={enabled && state.enabled && state.open}
      onOpenChange={(open) => { if (enabled) setState({ enabled, open }) }}>
      <RadixTooltip.Trigger asChild>{children}</RadixTooltip.Trigger>
      <RadixTooltip.Portal>
        <RadixTooltip.Content side="right" sideOffset={10} collisionPadding={8}
          className="vp-pop z-70 max-w-56 rounded-lg bg-primary px-2.5 py-1.5 text-caption break-words text-primary-fg shadow-pop">
          {label}
        </RadixTooltip.Content>
      </RadixTooltip.Portal>
    </RadixTooltip.Root>
  )
}
