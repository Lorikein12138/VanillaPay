import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import type { operations } from '@/api/schema'
import { ChannelIcon, ChannelOption } from '@/components/ui/payment'
import { OrderChannel } from '@/features/merchant/OrderParts'
import { createI18n } from '@/lib/i18n'

type Channel = operations['put_api_v1_merchant_collection_codes_channel']['parameters']['path']['channel']
const labels = {
  alipay: '支付宝',
  wxpay: '微信支付',
  qqpay: 'QQ 钱包',
} satisfies Record<Channel, string>

describe('channel icons', () => {
  it.each(Object.entries(labels))('renders a decorative brand shape for %s and preserves the button name', (channel, name) => {
    render(<ChannelOption channel={channel}>{name}</ChannelOption>)

    const button = screen.getByRole('button', { name })
    const icon = button.querySelector('[aria-hidden="true"]')
    const path = icon?.querySelector('svg path')
    expect(path?.getAttribute('d')).toMatch(/^M/)
    expect(screen.getByText(name, { exact: true })).toBeVisible()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it.each(['future-channel', 'constructor'])('renders a neutral square without a shape for %s', (channel) => {
    const { container } = render(<ChannelIcon channel={channel} />)

    expect(container.firstElementChild).toHaveClass('size-4', 'rounded-[4px]', '[--channel-color:var(--vp-text-3)]')
    expect(container.firstElementChild).toHaveAttribute('aria-hidden', 'true')
    expect(container.querySelector('svg')).toBeNull()
  })

  it('uses a muted glyph without a coloured tile in lists', () => {
    const i18n = createI18n('zh-CN')
    const { container } = render(<I18nextProvider i18n={i18n}><OrderChannel channel="alipay" /></I18nextProvider>)

    expect(screen.getByText('支付宝')).toBeVisible()
    const icon = container.querySelector('svg[aria-hidden="true"]')
    expect(icon).toHaveClass('text-text-3')
    expect(container.querySelector('[class*="--channel-color"]')).toBeNull()
  })

  it('renders nothing for an unknown channel in the plain tone', () => {
    const { container } = render(<ChannelIcon channel="future-channel" tone="plain" />)

    expect(container.firstChild).toBeNull()
  })

  it('displays an unassigned order channel without an icon', () => {
    const i18n = createI18n('zh-CN')
    const { container } = render(<I18nextProvider i18n={i18n}><OrderChannel channel={null} /></I18nextProvider>)

    expect(screen.getByText('未分配')).toBeVisible()
    expect(container.querySelector('[aria-hidden]')).toBeNull()
  })
})
