import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http } from 'msw'
import { HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { consoleApi } from '@/api/client'
import type { components } from '@/api/schema'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { createI18n } from '@/lib/i18n'
import { setupServer } from 'msw/node'

type S = components['schemas']
const settings: S['Settings'] = {
  version: 12,
  site: { site_name: 'VanillaPay', logo_asset_id: null },
  home: { mode: 'default', html: null, url: null },
  footer_legal: { items: [], terms: { zh_CN: '服务条款', en: '' }, privacy: { zh_CN: '隐私政策', en: '' } },
  mail: { host: 'smtp.example.test', port: 465, username: 'mailer', from_address: 'sender@example.test', from_name: 'Sender', tls_mode: 'tls', password_configured: true, masked_password: '••••••••' },
  security: { turnstile_site_key: '', allowed_hostname: '', restrict_email_domains: false, email_domains: [], ban_contact: { zh_CN: '', en: '' }, secret_configured: true },
  payment: { min_amount_cents: 100, max_amount_cents: 1000000, default_ttl_seconds: 300, max_ttl_seconds: 3600, float_min_cents: 0, float_max_cents: 99, float_default_cents: 0, float_direction: 'up' },
  billing: { merchant_pid: '', key_configured: false, merchant_id: null, credential_valid: true },
}
const server = setupServer()
let signedIn = false
const success = <T,>(data: T) => HttpResponse.json({ data })

beforeEach(() => {
  signedIn = true
  consoleApi.forgetSession()
  server.listen({ onUnhandledFrame: 'error' })
  server.use(
    http.get('*/api/v1/public/config', () => success({ site_name: 'VanillaPay', logo_url: '', turnstile: { configured: true, site_key: 'site', login_action: 'login', register_action: 'register' }, registration_open: true, email_domain_restriction: { enabled: false, domains: [] }, home: { mode: 'default', html: null, url: null }, footer: [], legal_revisions: { terms: 1, privacy: 1 } })),
    http.get('*/api/v1/console/me', () => signedIn ? success({ user: { id: '1', role: 'admin', username: 'admin', banned: false, banned_at: null, ban_reason: null, totp_enabled: false, locale: 'zh-CN', theme: 'light', created_at: new Date().toISOString() }, csrf_token: 'console-csrf', expires_at: new Date(Date.now() + 3600000).toISOString() }) : HttpResponse.json({ error: { code: 'UNAUTHENTICATED' } }, { status: 401 })),
    http.get('*/api/v1/console/settings', () => success(settings)),
  )
})
afterEach(() => { signedIn = false; server.resetHandlers(); server.close(); vi.useRealTimers() })

function mount(path = '/console/settings') {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) }), client = createQueryClient()
  return { router, ...render(<AppProviders queryClient={client} i18n={createI18n()}><RouterProvider router={router} /></AppProviders>) }
}

describe('console settings page', () => {
  it('uses the response version when saving the next group and keeps saved-secret fields omitted', async () => {
    const bodies: S['SettingsUpdateGroup'][] = []
    let revision = settings.version
    server.use(http.put('*/api/v1/console/settings/:group', async ({ request }) => {
      const body = await request.json() as S['SettingsUpdateGroup']
      bodies.push(body); revision += 1
      return success({ version: revision, group: 'site', value: body.value, affected_merchants: 0 })
    }))
    const actor = userEvent.setup(); mount('/console/settings?group=site')
    await actor.clear(await screen.findByLabelText('站点名称'))
    await actor.type(screen.getByLabelText('站点名称'), 'Updated platform')
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    await screen.findByText('站点信息已保存。')
    await actor.click(within(screen.getByRole('navigation', { name: '设置分组' })).getByRole('button', { name: '邮件发送' }))
    await actor.clear(screen.getByLabelText('邮件服务器'))
    await actor.type(screen.getByLabelText('邮件服务器'), 'smtp.updated.example.test')
    await actor.click(screen.getByRole('button', { name: '保存邮件设置' }))
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[0]?.expected_version).toBe(12)
    expect(bodies[1]?.expected_version).toBe(13)
    expect(bodies[1]?.value).not.toHaveProperty('password')
    expect(JSON.stringify(bodies)).not.toContain('••••••••')
  })

  it('submits a replacement secret as text and a confirmed clear as null', async () => {
    const bodies: S['SettingsUpdateGroup'][] = []
    server.use(http.put('*/api/v1/console/settings/mail', async ({ request }) => {
      const body = await request.json() as S['SettingsUpdateGroup']; bodies.push(body)
      return success({ version: 13 + bodies.length, group: 'mail', value: body.value, affected_merchants: 0 })
    }))
    const actor = userEvent.setup(); mount('/console/settings?group=mail')
    await actor.type(await screen.findByLabelText('邮件密码'), 'replacement-secret')
    await actor.click(screen.getByRole('button', { name: '保存邮件设置' }))
    await screen.findByText('邮件发送设置已保存。')
    expect(bodies[0]?.value).toHaveProperty('password', 'replacement-secret')
    await actor.click(screen.getByRole('checkbox', { name: '清除密钥' }))
    await actor.click(screen.getByRole('button', { name: '确认清除' }))
    await actor.click(screen.getByRole('button', { name: '保存邮件设置' }))
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[1]?.value).toHaveProperty('password', null)
  })

  it('falls back to the site group for an invalid search value', async () => {
    mount('/console/settings?group=unknown')
    expect(await screen.findByRole('heading', { name: /^站点信息$/ })).toBeVisible()
  })

  it('does not submit the previous payment amount when the yuan input is invalid', async () => {
    let saves = 0
    server.use(http.put('*/api/v1/console/settings/payment', () => { saves += 1; return success({ version: 13, group: 'payment', value: settings.payment, affected_merchants: 0 }) }))
    const actor = userEvent.setup(); mount('/console/settings?group=payment')
    await actor.clear(await screen.findByLabelText('最低金额（元）'))
    await actor.type(screen.getByLabelText('最低金额（元）'), '1.234')
    await actor.click(screen.getByRole('button', { name: '保存收款限制' }))
    expect(await screen.findByText('金额范围不符合平台限制。')).toBeVisible()
    expect(saves).toBe(0)
  })

  it('sends mutually exclusive home payload fields for each display mode', async () => {
    const values: Record<string, unknown>[] = []
    server.use(http.put('*/api/v1/console/settings/home', async ({ request }) => {
      const body = await request.json() as S['SettingsUpdateGroup']; values.push(body.value)
      return success({ version: 12 + values.length, group: 'home', value: body.value, affected_merchants: 0 })
    }))
    const actor = userEvent.setup(); mount('/console/settings?group=home')
    await screen.findByLabelText('首页展示方式')
    await actor.click(screen.getByRole('button', { name: '保存首页设置' }))
    await waitFor(() => expect(values).toHaveLength(1))
    await actor.selectOptions(screen.getByLabelText('首页展示方式'), 'html')
    await actor.type(screen.getByLabelText('HTML'), '<main>首页</main>')
    await actor.click(screen.getByRole('button', { name: '保存首页设置' }))
    await waitFor(() => expect(values).toHaveLength(2))
    await actor.selectOptions(screen.getByLabelText('首页展示方式'), 'url')
    await actor.type(screen.getByLabelText('首页地址'), 'https://home.example.test')
    await actor.click(screen.getByRole('button', { name: '保存首页设置' }))
    await waitFor(() => expect(values).toHaveLength(3))
    expect(values[0]).toMatchObject({ mode: 'default', html: null, url: null })
    expect(values[1]).toMatchObject({ mode: 'html', html: '<main>首页</main>', url: null })
    expect(values[2]).toMatchObject({ mode: 'url', html: null, url: 'https://home.example.test' })
  })

  it('reports how many merchant settings were adjusted after a payment bounds save', async () => {
    server.use(http.put('*/api/v1/console/settings/payment', async ({ request }) => {
      const body = await request.json() as S['SettingsUpdateGroup']
      return success({ version: 13, group: 'payment', value: body.value, affected_merchants: 3 })
    }))
    const actor = userEvent.setup(); mount('/console/settings?group=payment')
    await actor.click(await screen.findByRole('button', { name: '保存收款限制' }))
    expect(await screen.findByText('收款限制已保存，3 个商户的收款设置已按新范围调整。')).toBeVisible()
  })

  it('keeps a version-conflicted draft and retries with the fetched version', async () => {
    const versions: number[] = []
    let attempts = 0
    server.use(http.put('*/api/v1/console/settings/site', async ({ request }) => {
      const body = await request.json() as S['SettingsUpdateGroup']; versions.push(body.expected_version)
      if (attempts++ === 0) return HttpResponse.json({ error: { code: 'VERSION_CONFLICT', message: 'version conflict' } }, { status: 409 })
      return success({ version: 13, group: 'site', value: body.value, affected_merchants: 0 })
    }))
    const actor = userEvent.setup(); mount('/console/settings?group=site')
    const name = await screen.findByLabelText('站点名称')
    await actor.clear(name); await actor.type(name, 'Retained draft')
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    expect(await screen.findByLabelText('我已查看最新内容')).toBeVisible()
    expect(name).toHaveValue('Retained draft')
    await actor.click(screen.getByLabelText('我已查看最新内容'))
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    await waitFor(() => expect(versions).toHaveLength(2))
    expect(versions).toEqual([12, 12])
  })

  it('shows a stale billing credential as field errors instead of a version conflict', async () => {
    server.use(http.put('*/api/v1/console/settings/billing', () => HttpResponse.json({ error: { code: 'BILLING_CREDENTIAL_STALE', message: 'stale' } }, { status: 409 })))
    const actor = userEvent.setup(); mount('/console/settings?group=billing')
    await actor.type(await screen.findByLabelText('商户号'), '1234567890123456')
    await actor.click(screen.getByRole('button', { name: '保存订阅收款' }))
    expect(await screen.findAllByText('商户号与密钥不匹配，或该商户已停用/不存在。')).toHaveLength(2)
    expect(screen.queryByLabelText('我已查看最新内容')).not.toBeInTheDocument()
  })

  it('submits footer items in visual order with sequential sort values', async () => {
    let saved: S['SettingsUpdateGroup'] | undefined
    server.use(http.put('*/api/v1/console/settings/footer_legal', async ({ request }) => {
      saved = await request.json() as S['SettingsUpdateGroup']
      return success({ version: 13, group: 'footer_legal', value: saved.value, affected_merchants: 0 })
    }))
    const actor = userEvent.setup(); mount('/console/settings?group=footer_legal')
    await actor.click(await screen.findByRole('button', { name: '新增页脚项目' }))
    await actor.click(screen.getByRole('button', { name: '新增页脚项目' }))
    const labels = screen.getAllByLabelText('中文名称')
    await actor.type(labels[0]!, 'Alpha'); await actor.type(labels[1]!, 'Beta')
    await actor.click(screen.getAllByRole('button', { name: '上移' })[1]!)
    await actor.click(screen.getByRole('button', { name: '保存页脚与协议' }))
    await waitFor(() => expect(saved).toBeDefined())
    expect(saved!.value).toMatchObject({ items: [{ label: { zh_CN: 'Beta' }, sort: 0 }, { label: { zh_CN: 'Alpha' }, sort: 1 }] })
  })

  it('polls a test email until sent and stops after the terminal state', async () => {
    let polls = 0
    server.use(
      http.post('*/api/v1/console/settings/mail/test', () => success({ delivery_id: 'delivery-1', state: 'queued' })),
      http.get('*/api/v1/console/settings/mail/test/delivery-1', () => { polls += 1; return success({ id: 'delivery-1', state: 'sent', error_code: null }) }),
    )
    const actor = userEvent.setup(); mount('/console/settings?group=mail')
    await actor.type(await screen.findByPlaceholderText('收件人邮箱'), 'ops@example.test')
    await actor.click(screen.getByRole('button', { name: '发送测试邮件' }))
    expect(await screen.findByText('测试邮件已送达。', {}, { timeout: 5000 })).toBeVisible()
    expect(polls).toBe(1)
  })

  it('does not poll again after leaving the mail group', async () => {
    let polls = 0
    server.use(
      http.post('*/api/v1/console/settings/mail/test', () => success({ delivery_id: 'delivery-2', state: 'queued' })),
      http.get('*/api/v1/console/settings/mail/test/delivery-2', () => { polls += 1; return success({ id: 'delivery-2', state: 'sent', error_code: null }) }),
    )
    const actor = userEvent.setup(); mount('/console/settings?group=mail')
    await actor.type(await screen.findByPlaceholderText('收件人邮箱'), 'ops@example.test')
    await actor.click(screen.getByRole('button', { name: '发送测试邮件' }))
    await actor.click(within(screen.getByRole('navigation', { name: '设置分组' })).getByRole('button', { name: '首页' }))
    await new Promise((resolve) => setTimeout(resolve, 2200))
    expect(polls).toBe(0)
    expect(screen.queryByText('测试邮件已送达。')).not.toBeInTheDocument()
  })
})
