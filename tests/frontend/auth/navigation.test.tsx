import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StatusDot } from '@/components/StatusDot'
import { merchantNavigation } from '@/features/merchant/navigation'
import { loginDestination, safePath } from '@/lib/redirect'

describe('safe navigation', () => {
  it.each(['//evil.com', 'https://evil.com', 'javascript:alert(1)', '/\\evil.com', '/%2f%2fevil.com', '/%5cevil.com', '/%0a/evil.com', '/console', '/console/users', '/%63onsole', '/a/../console', '/login', 'dashboard', '', '/%'])('rejects %s', (path) => {
    expect(safePath(path)).toBeUndefined()
    expect(loginDestination(path, path)).toBe('/dashboard')
  })
  it('uses safe redirect before next_path and preserves query and hash', () => {
    expect(loginDestination('/orders?filter=paid#recent', '/dashboard')).toBe('/orders?filter=paid#recent')
    expect(loginDestination('//evil.com', '/banned')).toBe('/banned')
  })
  it('keeps the complete navigation sorted by label length and fixed product priority', () => {
    expect(merchantNavigation.map((item) => item.label)).toEqual(['概览', '订单', '通知', '收款码', '监控设备', '接入配置', '订阅套餐'])
    expect(merchantNavigation.map((item) => item.path)).toEqual(['/dashboard', '/orders', '/notifications', '/codes', '/devices', '/integration', '/subscription'])
    const lengths = merchantNavigation.map((item) => Array.from(item.label).length)
    expect(lengths).toEqual([...lengths].sort((a, b) => a - b))
    expect(merchantNavigation.filter((item) => item.enabled).map((item) => item.key)).toEqual(['overview', 'orders'])
  })
})

describe('status dots', () => {
  it.each(['ok', 'wait', 'info', 'off'] as const)('renders %s as a decorative 7px dot and readable text', (tone) => {
    render(<StatusDot tone={tone}>Status</StatusDot>)
    const label = screen.getByText('Status')
    expect(label).toBeVisible()
    expect(label.querySelector('[aria-hidden="true"]')).toHaveClass('size-[7px]', `bg-dot-${tone}`, 'rounded-full')
    expect(label).not.toHaveClass(`bg-dot-${tone}`)
  })
})
