import { chooseMenu } from '../portal-controls'
import userEvent from '@testing-library/user-event'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { delay, http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import type { components } from '@/api/schema'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { CheckoutPage } from '@/features/checkout/CheckoutPage'
import { ChannelOption } from '@/components/ui/payment'
import { checkoutDeadline } from '@/features/checkout/useCheckout'
import { returnToMerchant } from '@/features/checkout/return'
import { createI18n } from '@/lib/i18n'

vi.mock('@/features/checkout/return', () => ({ returnToMerchant: vi.fn() }))
type Checkout = components['schemas']['PaymentCheckoutDTO']
const path = '*/api/v1/checkout/order-1'
const success = (data: Checkout) => HttpResponse.json({ data })
const failure = (code: string, status: number) => HttpResponse.json({ error: { code, message: code } }, { status })
function order(overrides: Partial<Checkout> = {}): Checkout {
  return { trade_no: 'order-1', name: '月度会员', product_id: '', merchant: { username: '示例小店', id: '1' }, status: 'pending', reason: 'no_channel', original_cents: 3000, actual_cents: null, channel: null, available_channels: ['alipay', 'wxpay'], qr_image_url: null, expires_at: new Date(Date.now() + 300000).toISOString(), server_time: new Date().toISOString(), device_online: true, can_pay: false, return_url_signed: null, requires_exact_amount: true, ...overrides }
}
const locked = () => order({ channel: 'alipay', actual_cents: 2999, reason: null, can_pay: true, qr_image_url: '/media/original?signature=a%2Bb&expires=123' })
let value: Checkout, gets: number
const server = setupServer(http.get(path, () => { gets++; return success(value) }))
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
beforeEach(() => { value = order(); gets = 0; vi.mocked(returnToMerchant).mockClear(); Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' }) })
afterEach(() => { server.resetHandlers(); vi.useRealTimers() })
afterAll(() => server.close())
function mount(route = false) {
  const client = createQueryClient()
  return { client, ...render(<AppProviders queryClient={client} i18n={createI18n()}>{route ? <RouterProvider router={createAppRouter({ queryClient: client, history: createMemoryHistory({ initialEntries: ['/pay/order-1'] }) })} /> : <CheckoutPage tradeNo="order-1" />}</AppProviders>) }
}
async function tick(ms: number) { await act(async () => { await vi.advanceTimersByTimeAsync(ms) }) }

describe('public checkout', () => {
  it('loads without authentication, never locks automatically, then trusts GET rather than POST', async () => {
    let posts = 0
    server.use(http.post(path + '/channel', async ({ request }) => {
      posts++; expect(await request.json()).toEqual({ channel: 'alipay' })
      expect(request.headers.get('X-CSRF-Token')).toBeNull()
      value = locked()
      return success(order({ actual_cents: 1, channel: 'wxpay', available_channels: ['wxpay', 'alipay'] }))
    }))
    mount(true)
    const choose = await screen.findByRole('button', { name: '支付宝' })
    expect(posts).toBe(0)
    fireEvent.click(choose); fireEvent.click(choose)
    expect(await screen.findByRole('img', { name: '收款二维码' })).toHaveAttribute('src', value.qr_image_url)
    expect(posts).toBe(1); expect(gets).toBe(2)
    expect(screen.queryByRole('button', { name: '微信支付' })).not.toBeInTheDocument()
    expect(screen.getByText('已锁定')).toBeVisible()
    expect(screen.getByText('请务必支付 ¥29.99，金额不符将无法自动到账')).toBeVisible()
    expect(screen.getByText('¥30.00', { selector: 'del' })).toBeVisible()
  })
  it('requires a click even for one channel and does not select on keyboard focus', async () => {
    value.available_channels = ['alipay']; const post = vi.fn()
    server.use(http.post(path + '/channel', () => { post(); return success(value) }))
    mount(); (await screen.findByRole('button', { name: '支付宝' })).focus()
    expect(post).not.toHaveBeenCalled()
  })
  it('still warns about exact payment when actual equals original', async () => {
    value = { ...locked(), actual_cents: 3000 }; mount()
    expect(await screen.findByText('请按显示金额付款，金额不符将无法自动到账。')).toBeVisible()
    expect(document.querySelector('del')).toBeNull()
  })
  it.each([
    ['CHANNEL_LOCKED', 409, true], ['ORDER_NOT_PAYABLE', 409, true],
    ['CHANNEL_NOT_ALLOWED', 403, true], ['QR_UNAVAILABLE', 422, true],
    ['AMOUNT_CAPACITY_EXHAUSTED', 409, false], ['CSRF_FAILED', 403, false],
    ['RATE_LIMITED', 429, false], ['INTERNAL_ERROR', 503, false], ['VALIDATION_ERROR', 400, false],
  ])('handles %s and refreshes only when required', async (code, status, refresh) => {
    server.use(http.post(path + '/channel', () => { if (refresh) value = locked(); return failure(code, status) }))
    mount(); fireEvent.click(await screen.findByRole('button', { name: '支付宝' }))
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent(code === 'AMOUNT_CAPACITY_EXHAUSTED' ? '该支付方式当前繁忙' : refresh ? '订单或支付方式已发生变化' : '暂时无法完成请求')
    await waitFor(() => expect(gets).toBe(refresh ? 2 : 1))
    if (refresh) await screen.findByRole('img')
    else await waitFor(() => expect(screen.getByRole('button', { name: '支付宝' })).toBeEnabled())
  })
  it.each([-300000, 300000])('corrects client clock skew of %i ms', async (skew) => {
    const received = Date.now() + skew
    expect(checkoutDeadline(value.server_time, value.expires_at, received) - received).toBe(300000)
    vi.spyOn(Date, 'now').mockReturnValue(received)
    mount(); expect(await screen.findByText('5:00')).toBeVisible()
  })
  it('refreshes once at zero without locally deciding that the order expired', async () => {
    value.expires_at = value.server_time
    mount(); await screen.findByText('0:00'); await waitFor(() => expect(gets).toBe(2))
    expect(screen.queryByRole('heading', { name: '订单已过期' })).not.toBeInTheDocument()
  })
  it('polls while pending, pauses hidden, resumes visible/focused and cleans up on unmount', async () => {
    vi.useFakeTimers(); const mounted = mount(); await tick(100)
    await tick(3100); expect(gets).toBe(2)
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'hidden' })
    fireEvent(document, new Event('visibilitychange')); await tick(6100); expect(gets).toBe(2)
    Object.defineProperty(document, 'visibilityState', { configurable: true, value: 'visible' })
    fireEvent(document, new Event('visibilitychange')); await tick(50); expect(gets).toBe(3)
    fireEvent(window, new Event('focus')); await tick(50); expect(gets).toBe(4)
    mounted.unmount(); await tick(7000); expect(gets).toBe(4)
    fireEvent(window, new Event('focus')); await tick(50); expect(gets).toBe(4)
  })
  it.each(['paid', 'expired', 'refunded'])('stops polling in %s, even with a banned reason', async (status) => {
    vi.useFakeTimers(); mount(); await tick(100)
    value = order({ status, reason: 'merchant_banned' }); await tick(3100)
    expect(screen.getByRole('heading', { name: status === 'paid' ? '支付成功' : status === 'expired' ? '订单已过期' : '订单已退款' })).toBeVisible()
    await tick(10000); expect(gets).toBe(2)
    fireEvent(window, new Event('focus')); await tick(20); expect(gets).toBe(2)
  })
  it('shows a close message for a paid subscription without a return URL', async () => {
    value = order({ status: 'paid' }); mount()
    expect(await screen.findByText('可以关闭本页')).toBeVisible()
    expect(screen.queryByText(/^已支付/)).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '返回商户' })).not.toBeInTheDocument()
  })
  it.each([false, true])('returns using the exact signed URL (early click: %s)', async (early) => {
    const url = 'https://shop.example/return?a=%2B&sign=original%2Bvalue'
    value = order({ status: 'paid', return_url_signed: url }); vi.useFakeTimers()
    const mounted = mount(); await tick(100)
    if (early) fireEvent.click(screen.getByRole('button', { name: '返回商户' }))
    await tick(3000)
    expect(returnToMerchant).toHaveBeenCalledExactlyOnceWith(url)
    mounted.unmount(); await tick(5000); expect(returnToMerchant).toHaveBeenCalledTimes(1)
  })
  it('cancels automatic return when leaving the page', async () => {
    value = order({ status: 'paid', return_url_signed: 'https://shop.example/return' }); vi.useFakeTimers()
    const mounted = mount(); await tick(100); mounted.unmount(); await tick(4000)
    expect(returnToMerchant).not.toHaveBeenCalled()
  })
  it('同一订单的签名地址刷新后仍只能返回一次', async () => {
    vi.useFakeTimers()
    server.use(http.post(path + '/channel', async () => {
      await delay(5000); return success(locked())
    }))
    mount(); await tick(100)
    fireEvent.click(screen.getByRole('button', { name: '支付宝' })); await tick(100)
    value = order({ status: 'paid', return_url_signed: 'https://shop.example/return?timestamp=1&sign=one' })
    await tick(3100)
    expect(screen.getByRole('heading', { name: '支付成功' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: '返回商户' }))
    expect(returnToMerchant).toHaveBeenCalledExactlyOnceWith(value.return_url_signed)
    value = { ...value, return_url_signed: 'https://shop.example/return?timestamp=2&sign=two' }
    await tick(2100); expect(gets).toBe(3)
    fireEvent.click(screen.getByRole('button', { name: '返回商户' }))
    expect(returnToMerchant).toHaveBeenCalledTimes(1)
    await tick(3100); expect(returnToMerchant).toHaveBeenCalledTimes(1)
  })
  it('does not return again when the signed URL changes after automatic return', async () => {
    vi.useFakeTimers()
    server.use(http.post(path + '/channel', async () => {
      await delay(8000); return success(locked())
    }))
    mount(); await tick(100)
    fireEvent.click(screen.getByRole('button', { name: '支付宝' })); await tick(100)
    const first = 'https://shop.example/return?timestamp=1&sign=one'
    value = order({ status: 'paid', return_url_signed: first })
    await tick(3100); await tick(3100)
    expect(returnToMerchant).toHaveBeenCalledExactlyOnceWith(first)
    value = { ...value, return_url_signed: 'https://shop.example/return?timestamp=2&sign=two' }
    await tick(2100); expect(gets).toBe(3)
    await tick(3100); expect(returnToMerchant).toHaveBeenCalledTimes(1)
  })
  it('keeps the original deadline and returns once using the latest exact URL', async () => {
    vi.useFakeTimers()
    server.use(http.post(path + '/channel', async () => {
      await delay(4500); return success(locked())
    }))
    mount(); await tick(100)
    fireEvent.click(screen.getByRole('button', { name: '支付宝' })); await tick(100)
    value = order({ status: 'paid', return_url_signed: 'https://shop.example/return?timestamp=1&sign=one' })
    await tick(3100)
    expect(screen.getByText('3 秒后返回商户')).toBeVisible()
    const latest = 'https://shop.example/return?timestamp=2&sign=a%2Bb&extra=%2F'
    value = { ...value, return_url_signed: latest }
    await tick(1500); expect(gets).toBe(3)
    expect(returnToMerchant).not.toHaveBeenCalled()
    expect(screen.queryByText('3 秒后返回商户')).not.toBeInTheDocument()
    await tick(1600)
    expect(returnToMerchant).toHaveBeenCalledExactlyOnceWith(latest)
    await tick(4000); expect(returnToMerchant).toHaveBeenCalledTimes(1)
  })
  it('ignores a late paid response after leaving the page', async () => {
    vi.useFakeTimers()
    server.use(http.post(path + '/channel', async () => {
      await delay(5000); return success(locked())
    }))
    const mounted = mount(); await tick(100)
    fireEvent.click(screen.getByRole('button', { name: '支付宝' })); await tick(100)
    value = order({ status: 'paid', return_url_signed: 'https://shop.example/return' })
    await tick(3100)
    mounted.unmount(); await tick(10000)
    expect(gets).toBe(2)
    expect(returnToMerchant).not.toHaveBeenCalled()
  })
  it('never falls back to the original amount when a locked actual amount is null', async () => {
    value = { ...locked(), actual_cents: null, can_pay: false, reason: 'qr_disabled', qr_image_url: null }
    mount(); expect(await screen.findByText('实付金额')).toBeVisible()
    expect(screen.getByText('—')).toBeVisible()
    expect(screen.queryByText('¥30.00')).not.toBeInTheDocument()
  })
  it.each(['device_offline', 'qr_disabled'])('immediately removes an existing QR when GET changes to %s', async (reason) => {
    vi.useFakeTimers(); value = locked(); mount(); await tick(100)
    expect(screen.getByRole('img', { name: '收款二维码' })).toBeVisible()
    // Even a stale non-null image URL must not keep the QR visible.
    value = { ...value, reason, can_pay: false, device_online: reason !== 'device_offline' }
    await tick(3100)
    expect(screen.queryByRole('img', { name: '收款二维码' })).not.toBeInTheDocument()
  })
  it('aborts an in-flight GET on unmount and ignores its late paid response', async () => {
    vi.useFakeTimers()
    let requestSignal: AbortSignal | undefined
    server.use(http.get(path, async ({ request }) => {
      gets++; requestSignal = request.signal
      await delay(2000)
      return success(order({ status: 'paid', return_url_signed: 'https://shop.example/return' }))
    }))
    const mounted = mount(); await tick(100)
    expect(requestSignal).toBeDefined()
    mounted.unmount(); expect(requestSignal!.aborted).toBe(true)
    await tick(7000)
    expect(gets).toBe(1); expect(returnToMerchant).not.toHaveBeenCalled()
  })
  it('does not let a slow old poll overwrite the fresh GET after channel selection', async () => {
    vi.useFakeTimers()
    let oldSignal: AbortSignal | undefined
    server.use(http.get(path, async ({ request }) => {
      gets++
      if (gets === 2) {
        oldSignal = request.signal
        const stale = order()
        await delay(5000)
        return success(stale)
      }
      return success(value)
    }), http.post(path + '/channel', () => { value = locked(); return success(value) }))
    mount(); await tick(100); await tick(3100)
    expect(oldSignal).toBeDefined()
    fireEvent.click(screen.getByRole('button', { name: '支付宝' })); await tick(100)
    expect(oldSignal!.aborted).toBe(true)
    expect(screen.getByRole('img', { name: '收款二维码' })).toBeVisible()
    await tick(5100)
    expect(screen.getByRole('img', { name: '收款二维码' })).toHaveAttribute('src', value.qr_image_url)
    expect(screen.queryByRole('button', { name: '支付宝' })).not.toBeInTheDocument()
  })
  it('corrects the clock using response receipt time even when GET is delayed', async () => {
    vi.useFakeTimers()
    const sentAt = Date.now()
    server.use(http.get(path, async () => { await delay(2000); return success(value) }))
    mount(); await tick(2100)
    expect(Date.now() - sentAt).toBe(2100)
    expect(screen.getByText('5:00')).toBeVisible()
    await tick(1000); expect(screen.getByText('4:59')).toBeVisible()
  })
  it.each([
    ['merchant_banned', '商户暂停收款，请联系商户。'],
    ['device_offline', '收款设备离线，请稍后或联系商户。页面会自动刷新。'],
    ['qr_disabled', '该支付方式暂不可用，请稍后或联系商户。页面会自动刷新。'],
  ])('explains %s without exposing a QR', async (reason, message) => {
    value = order({ reason }); mount(); expect(await screen.findByText(message)).toBeVisible()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })
  it('labels the channel group without repeating an informational notice', async () => {
    mount(); expect(await screen.findByRole('group', { name: '选择支付方式' })).toBeVisible()
    expect(screen.queryByText('请选择支付方式。')).not.toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(screen.getByText('订单号')).toBeVisible()
  })
  it('shows the paid amount and localized channel once, including English', async () => {
    value = { ...locked(), status: 'paid' }; mount()
    expect(await screen.findByText('已支付 ¥29.99 · 支付宝')).toBeVisible()
    await chooseMenu(userEvent.setup(), '语言', 'English')
    expect(await screen.findByText('Paid ¥29.99 · Alipay')).toBeVisible()
    expect(screen.getByText('You may close this page.')).toBeVisible()
  })
  it.each([false, true])('renders a channel option selected=%s with disabled behavior', (selected) => {
    const clicked = vi.fn()
    const view = render(<ChannelOption channel="wxpay" selected={selected} onClick={clicked}>微信支付</ChannelOption>)
    const option = screen.getByRole('button', { name: '微信支付' })
    expect(option).toHaveAttribute('aria-pressed', String(selected))
    fireEvent.click(option); expect(clicked).toHaveBeenCalledTimes(1)
    view.rerender(<ChannelOption channel="wxpay" selected={selected} disabled onClick={clicked}>微信支付</ChannelOption>)
    expect(option).toBeDisabled(); fireEvent.click(option); expect(clicked).toHaveBeenCalledTimes(1)
  })
  it('handles a missing order and does not poll', async () => {
    server.use(http.get(path, () => { gets++; return failure('NOT_FOUND', 404) }))
    mount(); await screen.findByRole('heading', { name: '订单不存在' }); vi.useFakeTimers()
    await tick(10000); expect(gets).toBe(1)
  })
  it('hides an old QR if a later GET reports the order missing', async () => {
    value = locked(); vi.useFakeTimers(); mount(); await tick(100)
    expect(screen.getByRole('img')).toBeVisible()
    server.use(http.get(path, () => { gets++; return failure('NOT_FOUND', 404) }))
    await tick(3100)
    expect(screen.getByRole('heading', { name: '订单不存在' })).toBeVisible()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    await tick(6000); expect(gets).toBe(2)
  })
  it('retries an initial network/server error', async () => {
    server.use(http.get(path, () => failure('INTERNAL_ERROR', 503)))
    mount(); fireEvent.click(await screen.findByRole('button', { name: '重试' }))
    server.use(http.get(path, () => success(value)))
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(await screen.findByText('月度会员')).toBeVisible()
  })
  it('aborts an in-flight channel selection on unmount without starting a follow-up GET', async () => {
    let requestSignal: AbortSignal | undefined
    server.use(http.post(path + '/channel', async ({ request }) => {
      requestSignal = request.signal
      await delay(100)
      return success(locked())
    }))
    const mounted = mount(); fireEvent.click(await screen.findByRole('button', { name: '支付宝' }))
    await waitFor(() => expect(requestSignal).toBeDefined())
    mounted.unmount()
    expect(requestSignal!.aborted).toBe(true)
    await new Promise((resolve) => setTimeout(resolve, 150))
    expect(gets).toBe(1)
  })
})
