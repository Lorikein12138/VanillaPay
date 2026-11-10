import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

export async function chooseSelect(user: ReturnType<typeof userEvent.setup>, control: HTMLElement, value: string) {
  control.focus()
  await user.keyboard('{Enter}')
  await screen.findByRole('listbox')
  const option = await waitFor(() => {
    const item = screen.getAllByRole('option').find((candidate) => candidate.dataset.value === value)
    if (!item) throw new Error('Select option missing: ' + value)
    return item
  })
  await user.click(option)
}

export async function chooseMenu(user: ReturnType<typeof userEvent.setup>, label: string, option: string) {
  await user.click(await screen.findByRole('button', { name: label }))
  await user.click(await screen.findByRole('menuitemradio', { name: option }))
}
