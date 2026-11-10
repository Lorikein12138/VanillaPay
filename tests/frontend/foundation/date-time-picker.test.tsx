import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import { DateTimeField } from '@/components/ui/date-time-picker'
import { createI18n } from '@/lib/i18n'

function Harness({ initial, onChange }: { initial: string; onChange: (value: string) => void }) {
  const [value, setValue] = useState(initial)
  return <DateTimeField label="开始时间" value={value} onValueChange={(next) => { setValue(next); onChange(next) }} />
}

describe('date time picker text box', () => {
  it('shows a space instead of the ISO separator while keeping the stored value', () => {
    const changes = vi.fn()
    render(<I18nextProvider i18n={createI18n('zh-CN')}><Harness initial="2026-11-10T08:30:00" onChange={changes} /></I18nextProvider>)
    const input = screen.getByLabelText('开始时间')
    expect(input).toHaveValue('2026-11-10 08:30:00')
    expect(input).toHaveAttribute('placeholder', 'YYYY-MM-DD HH:mm:ss')

    fireEvent.change(input, { target: { value: '2026-11-11 09:15' } })
    expect(changes).toHaveBeenLastCalledWith('2026-11-11T09:15')
    expect(input).toHaveValue('2026-11-11 09:15')

    fireEvent.change(input, { target: { value: '2026-11-12T10:00:00' } })
    expect(changes).toHaveBeenLastCalledWith('2026-11-12T10:00:00')
    expect(input).toHaveValue('2026-11-12 10:00:00')

    fireEvent.change(input, { target: { value: '2026-11' } })
    expect(changes).toHaveBeenLastCalledWith('2026-11')
  })
})
