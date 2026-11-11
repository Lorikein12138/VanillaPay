import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, it } from 'vitest'
import { Tooltip, TooltipProvider } from '@/components/ui/tooltip'

function subject(enabled: boolean) {
  return (
    <TooltipProvider delayDuration={0}>
      <Tooltip enabled={enabled} label="Account details"><button type="button">Account</button></Tooltip>
      <button type="button">Other</button>
    </TooltipProvider>
  )
}

it.each(['hover', 'focus'] as const)('does not replay disabled %s when a tooltip is enabled', async (interaction) => {
  const user = userEvent.setup()
  const view = render(subject(false))
  const trigger = screen.getByRole('button', { name: 'Account' })
  if (interaction === 'hover') {
    await user.hover(trigger)
    await user.unhover(trigger)
  } else {
    await user.tab()
    await user.tab()
  }
  view.rerender(subject(true))
  expect(trigger).toHaveAttribute('data-state', 'closed')
  expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  if (interaction === 'hover') await user.hover(trigger)
  else await user.tab({ shift: true })
  expect(await screen.findByRole('tooltip')).toHaveTextContent('Account details')
  view.rerender(subject(false))
  view.rerender(subject(true))
  expect(trigger).toHaveAttribute('data-state', 'closed')
  expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
})
