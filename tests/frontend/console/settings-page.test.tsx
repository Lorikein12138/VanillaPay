import { focusManager } from '@tanstack/react-query'
import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http } from 'msw'
import { HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { consoleApi } from '@/api/client'
import type { components } from '@/api/schema'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { byteLength, newFooterItem, type SettingsGroup } from '@/features/console/settings-data'
import { createI18n } from '@/lib/i18n'
import { publicConfigQuery } from '@/lib/public-config'
import { setupServer } from 'msw/node'
import { controlPublicConfigReads, defaultPublicConfig } from '../public-config-fixture'

vi.mock('@/features/auth/Turnstile', () => ({
  Turnstile: ({ siteKey, action, onToken }: { siteKey: string; action: string; onToken: (token: string) => void }) => (
    <button type="button" data-site-key={siteKey} data-action={action} onClick={() => onToken(crypto.randomUUID())}>完成人机验证</button>
  ),
}))

type S = components['schemas']
const settings: S['Settings'] = {
  version: 12,
  site: { site_name: 'VanillaPay', logo_asset_id: null, favicon_asset_id: null },
  home: { mode: 'default', html: null, url: null },
  footer_legal: { items: [], terms: { zh_CN: '服务条款', en: '' }, privacy: { zh_CN: '隐私政策', en: '' } },
  mail: {
    host: 'smtp.example.test',
    port: 465,
    username: 'mailer',
    from_address: 'sender@example.test',
    from_name: 'Sender',
    tls_mode: 'tls',
    password_configured: true,
    masked_password: '••••••••',
  },
  security: {
    turnstile_site_key: '',
    turnstile_enabled: false,
    restrict_email_domains: false,
    email_domains: [],
    ban_contact: { zh_CN: '', en: '' },
    secret_configured: true,
  },
  payment: {
    min_amount_cents: 100,
    max_amount_cents: 1000000,
    default_ttl_seconds: 300,
    max_ttl_seconds: 3600,
    float_min_cents: 0,
    float_max_cents: 99,
    float_default_cents: 0,
    float_direction: 'up',
  },
  billing: { merchant_pid: '', key_configured: false, merchant_id: null, credential_valid: true },
}
const server = setupServer()
let signedIn = false
let current: S['Settings']
const success = <T,>(data: T) => HttpResponse.json({ data })
const conflictResponse = () => HttpResponse.json({ error: { code: 'VERSION_CONFLICT' } }, { status: 409 })

function saveResponse(group: SettingsGroup, body: S['SettingsUpdateGroup'], affected = 0) {
  const fields = { ...body.value } as Record<string, unknown>
  if ('password' in fields) {
    current.mail.password_configured = fields.password !== null
    current.mail.masked_password = current.mail.password_configured ? '••••••••' : ''
    delete fields.password
  }
  delete fields.turnstile_token
  Object.assign(current[group], fields)
  current.version += 1
  return success({ version: current.version, group, value: fields, affected_merchants: affected })
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

function groupButton(name: string) {
  return within(screen.getByRole('navigation', { name: '设置分组' })).getByRole('button', { name })
}

function recordSaves() {
  const bodies: S['SettingsUpdateGroup'][] = []
  server.use(
    http.put('*/api/v1/console/settings/:group', async ({ params, request }) => {
      const body = (await request.json()) as S['SettingsUpdateGroup']
      bodies.push(body)
      return saveResponse(params.group as SettingsGroup, body)
    }),
  )
  return bodies
}

beforeEach(() => {
  signedIn = true
  current = structuredClone(settings)
  consoleApi.forgetSession()
  server.listen({ onUnhandledFrame: 'error' })
  server.use(
    http.get('*/api/v1/public/config', () =>
      success({
        site_name: 'VanillaPay',
        logo_url: '', favicon_url: '/favicon.ico?v=default',
        turnstile: { enabled: true, site_key: 'site', login_action: 'login', register_action: 'register' },
        registration_open: true,
        email_domain_restriction: { enabled: false, domains: [] },
        home: { mode: 'default', html: null, url: null },
        footer: [],
        legal_revisions: { terms: 1, privacy: 1 },
      }),
    ),
    http.get('*/api/v1/console/me', () =>
      signedIn
        ? success({
            user: {
              id: '1',
              role: 'admin',
              username: 'admin',
              banned: false,
              banned_at: null,
              ban_reason: null,
              totp_enabled: false,
              locale: 'zh-CN',
              theme: 'light',
              created_at: new Date().toISOString(),
            },
            csrf_token: 'console-csrf',
            expires_at: new Date(Date.now() + 3600000).toISOString(),
          })
        : HttpResponse.json({ error: { code: 'UNAUTHENTICATED' } }, { status: 401 }),
    ),
    http.get('*/api/v1/console/settings', () => success(current)),
  )
})
afterEach(() => {
  signedIn = false
  server.resetHandlers()
  server.close()
  focusManager.setFocused(undefined)
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function mount(path = '/console/settings') {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) }),
    client = createQueryClient()
  return {
    router,
    client,
    ...render(
      <AppProviders queryClient={client} i18n={createI18n()}>
        <RouterProvider router={router} />
      </AppProviders>,
    ),
  }
}

describe('console settings page', () => {
  it.each([
    ['site', '保存站点信息', '站点信息已保存。'],
    ['home', '保存首页设置', '首页设置已保存。'],
    ['footer_legal', '保存页脚与协议', '页脚与协议已保存。'],
  ] as const)('refreshes the shared public config after saving %s', async (group, button, saved) => {
    const bodies = recordSaves()
    const versions: number[] = []
    server.use(http.get('*/api/v1/public/config', () => {
      versions.push(current.version)
      return success({ ...defaultPublicConfig, site_name: `Platform ${current.version}` })
    }))
    mount(`/console/settings?group=${group}`)
    await waitFor(() => expect(document.title).toBe('Platform 12'))
    const save = await screen.findByRole('button', { name: button })
    fireEvent.click(save)
    expect(await screen.findByText(saved)).toBeVisible()
    await waitFor(() => expect(document.title).toBe('Platform 13'))
    expect(versions).toEqual([12, 13])
    expect(bodies).toHaveLength(1)
  })

  it('refreshes the published captcha and email domain policy after saving security', async () => {
    const bodies = recordSaves()
    server.use(http.get('*/api/v1/public/config', () => {
      const security = current.security
      return success({
        ...defaultPublicConfig,
        turnstile: { ...defaultPublicConfig.turnstile, enabled: security.turnstile_enabled && security.secret_configured, site_key: security.turnstile_site_key },
        email_domain_restriction: { enabled: security.restrict_email_domains, domains: security.email_domains },
      })
    }))
    const { client } = mount('/console/settings?group=security')
    const published = () => client.getQueryData(publicConfigQuery.queryKey)
    await waitFor(() => expect(published()).toMatchObject({
      turnstile: { site_key: '' }, email_domain_restriction: { enabled: false, domains: [] },
    }))
    fireEvent.change(await screen.findByLabelText('人机验证站点密钥'), { target: { value: 'updated-site' } })
    fireEvent.click(screen.getByRole('checkbox', { name: '要求人机验证（Turnstile）' }))
    expect(screen.getByRole('button', { name: '保存安全设置' })).toBeDisabled()
    const captcha = screen.getByRole('button', { name: '完成人机验证' })
    expect(captcha).toHaveAttribute('data-site-key', 'updated-site')
    expect(captcha).toHaveAttribute('data-action', 'settings_security')
    fireEvent.click(captcha)
    fireEvent.click(screen.getByRole('checkbox', { name: '限制注册邮箱域名' }))
    fireEvent.change(screen.getByLabelText('允许的邮箱域名（每行一个）'), { target: { value: 'merchant.example' } })
    fireEvent.click(screen.getByRole('button', { name: '保存安全设置' }))

    expect(await screen.findByText('安全与注册设置已保存。')).toBeVisible()
    expect(bodies).toHaveLength(1)
    expect(bodies[0]?.value).toHaveProperty('turnstile_token')
    await waitFor(() => expect(published()).toMatchObject({
      turnstile: { enabled: true, site_key: 'updated-site' },
      email_domain_restriction: { enabled: true, domains: ['merchant.example'] },
    }))
  })

  it('cancels the initial public config GET after saving security and never caches its snapshot', async () => {
    const bodies = recordSaves()
    const { handler, reads, releaseFirst, releaseSecond, firstReturned } = controlPublicConfigReads(() => {
      const security = current.security
      return {
        ...defaultPublicConfig,
        turnstile: { ...defaultPublicConfig.turnstile, enabled: security.turnstile_enabled && security.secret_configured, site_key: security.turnstile_site_key },
        email_domain_restriction: { enabled: security.restrict_email_domains, domains: security.email_domains },
      }
    })
    server.use(handler)
    const { client } = mount('/console/settings?group=security')
    const published = () => client.getQueryData(publicConfigQuery.queryKey)
    const cached: S['PublicConfig'][] = []
    const unsubscribe = client.getQueryCache().subscribe(() => {
      const value = published()
      if (value) cached.push(value)
    })
    try {
      await waitFor(() => expect(reads).toHaveLength(1))
      expect(published()).toBeUndefined()
      fireEvent.change(await screen.findByLabelText('人机验证站点密钥'), { target: { value: 'updated-site' } })
      fireEvent.click(screen.getByRole('checkbox', { name: '限制注册邮箱域名' }))
      fireEvent.change(screen.getByLabelText('允许的邮箱域名（每行一个）'), { target: { value: 'merchant.example' } })
      fireEvent.click(screen.getByRole('button', { name: '保存安全设置' }))

      expect(await screen.findByText('安全与注册设置已保存。')).toBeVisible()
      expect(bodies).toHaveLength(1)
      await waitFor(() => expect(reads).toHaveLength(2))
      expect(reads[0]?.signal.aborted).toBe(true)
      expect(reads[0]?.data.email_domain_restriction).toEqual({ enabled: false, domains: [] })
      await act(async () => {
        releaseFirst()
        await firstReturned
      })
      expect(published()).toBeUndefined()
      releaseSecond()

      await waitFor(() => expect(published()).toMatchObject({
        turnstile: { enabled: false, site_key: 'updated-site' },
        email_domain_restriction: { enabled: true, domains: ['merchant.example'] },
      }))
      expect(cached).not.toContainEqual(reads[0]?.data)
      expect(reads).toHaveLength(2)
    } finally {
      unsubscribe()
      releaseFirst()
      releaseSecond()
    }
  })

  it('requires review after another administrator replaces the password queued for clearing', async () => {
    const bodies = recordSaves()
    let rejected: S['SettingsUpdateGroup'] | undefined
    server.use(
      http.put(
        '*/api/v1/console/settings/mail',
        async ({ request }) => {
          rejected = (await request.json()) as S['SettingsUpdateGroup']
          current.version++
          return conflictResponse()
        },
        { once: true },
      ),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=mail')
    await actor.click(await screen.findByRole('button', { name: '清除邮件密码' }))
    await actor.click(screen.getByRole('button', { name: '确认清除' }))
    const save = screen.getByRole('button', { name: '保存邮件设置' })
    await actor.click(save)

    const review = await screen.findByLabelText('我已查看最新内容')
    expect(rejected).toMatchObject({ expected_version: 12, value: { password: null } })
    expect(screen.getByText('已配置', { exact: true })).toBeVisible()
    expect(save).toBeDisabled()
    await actor.click(save)
    expect(bodies).toHaveLength(0)

    await actor.click(review)
    await actor.click(save)
    await screen.findByText('邮件发送设置已保存。')
    expect(bodies).toMatchObject([{ expected_version: 13, value: { password: null } }])
  })

  it('flags a pending secret replacement on focus while an unchanged ordinary group can save', async () => {
    const bodies = recordSaves()
    const actor = userEvent.setup()
    mount()
    fireEvent.change(await screen.findByLabelText('站点名称'), { target: { value: 'Site draft' } })
    await actor.click(groupButton('邮件发送'))
    const password = screen.getByLabelText('邮件密码')
    fireEvent.change(password, { target: { value: crypto.randomUUID() } })

    current.version++
    current.mail.password_configured = false
    current.mail.masked_password = ''
    act(() => {
      focusManager.setFocused(false)
      focusManager.setFocused(true)
    })
    await screen.findByLabelText('我已查看最新内容')
    expect(password).toHaveAccessibleDescription('尚未配置。')
    expect(screen.getByRole('button', { name: '保存邮件设置' })).toBeDisabled()
    fireEvent.change(password, { target: { value: crypto.randomUUID() } })
    expect(screen.getByRole('button', { name: '保存邮件设置' })).toBeDisabled()

    await actor.click(groupButton('站点信息 有未保存修改'))
    expect(screen.queryByLabelText('我已查看最新内容')).not.toBeInTheDocument()
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    await screen.findByText('站点信息已保存。')
    expect(bodies).toMatchObject([{ expected_version: 13, value: { site_name: 'Site draft' } }])
    await actor.click(groupButton('邮件发送 有未保存修改'))
    expect(screen.getByLabelText('我已查看最新内容')).not.toBeChecked()
    expect(screen.getByRole('button', { name: '保存邮件设置' })).toBeDisabled()
  })

  it('points invalid URL escapes at the affected link and gives a specific fallback for server validation', async () => {
    const actor = userEvent.setup()
    let puts = 0
    server.use(
      http.put('*/api/v1/console/settings/footer_legal', () => {
        puts++
        return HttpResponse.json({ error: { code: 'VALIDATION_ERROR' } }, { status: 400 })
      }),
    )
    current.footer_legal.items = [
      { ...newFooterItem(), label: { zh_CN: '帮助', en: '' }, url: 'https://example.test/help' },
      { ...newFooterItem(), label: { zh_CN: '支持', en: '' }, url: 'mailto:ops@example.test' },
    ]
    mount('/console/settings?group=footer_legal')
    const [first, second] = await screen.findAllByLabelText('链接地址')
    const save = screen.getByRole('button', { name: '保存页脚与协议' })
    fireEvent.change(second!, { target: { value: 'https://example.test/docs/%zz' } })
    await actor.click(save)
    expect(second).toHaveAccessibleDescription('请检查页脚链接格式。')
    expect(first).not.toHaveAccessibleDescription()
    expect(screen.getAllByLabelText('中文名称')[1]).not.toHaveAccessibleDescription()
    expect(puts).toBe(0)

    fireEvent.change(second!, { target: { value: 'https://example.test/docs/%E4%B8%AD' } })
    await actor.click(save)
    expect(await screen.findByText('请检查页脚链接格式。')).toBeVisible()
    expect(second).toHaveValue('https://example.test/docs/%E4%B8%AD')
    expect(puts).toBe(1)
  })

  it('after a site conflict saves an untouched security group with the latest values', async () => {
    const bodies = recordSaves()
    let rejected: S['SettingsUpdateGroup'] | undefined
    server.use(
      http.put(
        '*/api/v1/console/settings/site',
        async ({ request }) => {
          rejected = (await request.json()) as S['SettingsUpdateGroup']
          current.version++
          current.site.site_name = 'Other administrator'
          current.security.restrict_email_domains = true
          current.security.email_domains = ['other.example']
          return conflictResponse()
        },
        { once: true },
      ),
    )
    const actor = userEvent.setup()
    mount()
    fireEvent.change(await screen.findByLabelText('站点名称'), { target: { value: 'Local draft' } })
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    expect(await screen.findByText('Other administrator')).toBeVisible()
    expect(rejected?.expected_version).toBe(12)
    const saveSite = screen.getByRole('button', { name: '保存站点信息' })
    expect(saveSite).toBeDisabled()
    await actor.click(screen.getByLabelText('我已查看最新内容'))
    await actor.click(saveSite)
    await screen.findByText('站点信息已保存。')
    await actor.click(groupButton('安全与注册'))
    expect(screen.getByLabelText('限制注册邮箱域名')).toBeChecked()
    await actor.click(screen.getByRole('button', { name: '保存安全设置' }))
    await screen.findByText('安全与注册设置已保存。')
    expect(bodies).toMatchObject([
      { expected_version: 13, value: { site_name: 'Local draft' } },
      { expected_version: 14, value: { restrict_email_domains: true, email_domains: ['other.example'] } },
    ])
  })

  it('checks each draft on focus and only acknowledges the selected group, merging untouched fields', async () => {
    const bodies = recordSaves()
    const actor = userEvent.setup()
    mount()
    fireEvent.change(await screen.findByLabelText('站点名称'), { target: { value: 'Draft site' } })
    await actor.click(groupButton('邮件发送'))
    fireEvent.change(screen.getByLabelText('邮件服务器'), { target: { value: 'smtp.draft.test' } })

    current.version++
    current.site.logo_asset_id = 'logo-from-server'
    current.mail.from_name = 'Sender from server'
    act(() => {
      focusManager.setFocused(false)
      focusManager.setFocused(true)
    })
    await screen.findByText('Sender from server')
    await actor.click(screen.getByLabelText('我已查看最新内容'))
    expect(screen.getByLabelText('发件人名称')).toHaveValue('Sender from server')
    await actor.click(groupButton('站点信息 有未保存修改'))
    expect(screen.getByLabelText('我已查看最新内容')).not.toBeChecked()
    expect(screen.getByRole('button', { name: '保存站点信息' })).toBeDisabled()
    await actor.click(groupButton('邮件发送 有未保存修改'))
    expect(screen.queryByLabelText('我已查看最新内容')).not.toBeInTheDocument()
    await actor.click(screen.getByRole('button', { name: '保存邮件设置' }))
    await screen.findByText('邮件发送设置已保存。')
    await actor.click(groupButton('站点信息 有未保存修改'))
    await actor.click(screen.getByLabelText('我已查看最新内容'))
    expect(screen.getByRole('img', { name: '站点标识' })).toHaveAttribute('src', '/media/logo-from-server')
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    await screen.findByText('站点信息已保存。')
    expect(bodies).toMatchObject([
      { expected_version: 13, value: { host: 'smtp.draft.test', from_name: 'Sender from server' } },
      { expected_version: 14, value: { site_name: 'Draft site', logo_asset_id: 'logo-from-server', favicon_asset_id: null } },
    ])
  })

  it('keeps drafts when a conflict refresh fails and retries at the newer version without a cross-group conflict', async () => {
    const bodies = recordSaves()
    server.use(
      http.put(
        '*/api/v1/console/settings/site',
        () => {
          current.version++
          current.mail.from_name = 'Other group changed'
          server.use(
            http.get('*/api/v1/console/settings', () => new HttpResponse(null, { status: 503 }), {
              once: true,
            }),
          )
          return conflictResponse()
        },
        { once: true },
      ),
    )
    const actor = userEvent.setup()
    mount()
    const name = await screen.findByLabelText('站点名称')
    fireEvent.change(name, { target: { value: 'Retry draft' } })
    const save = screen.getByRole('button', { name: '保存站点信息' })
    await actor.click(save)
    await screen.findByText(/未能加载最新设置/)
    expect(name).toHaveValue('Retry draft')
    expect(save).toBeDisabled()
    await actor.click(screen.getByRole('button', { name: '重新加载设置' }))
    await waitFor(() => expect(save).toBeEnabled())
    expect(screen.queryByLabelText('我已查看最新内容')).not.toBeInTheDocument()
    await actor.click(save)
    await screen.findByText('站点信息已保存。')
    expect(bodies).toMatchObject([{ expected_version: 13, value: { site_name: 'Retry draft' } }])
  })

  it('freezes the form and group navigation during PUT and retains another group’s fields and secret', async () => {
    const pending = deferred<void>()
    const started = deferred<void>()
    server.use(
      http.put('*/api/v1/console/settings/site', async ({ request }) => {
        const body = (await request.json()) as S['SettingsUpdateGroup']
        started.resolve()
        await pending.promise
        return saveResponse('site', body)
      }),
    )
    const actor = userEvent.setup()
    const secret = crypto.randomUUID()
    const { router } = mount('/console/settings?group=mail')
    fireEvent.change(await screen.findByLabelText('邮件服务器'), { target: { value: 'smtp.unsaved.test' } })
    fireEvent.change(screen.getByLabelText('邮件密码'), { target: { value: secret } })
    await actor.click(groupButton('站点信息'))
    const name = screen.getByLabelText('站点名称')
    fireEvent.change(name, { target: { value: 'Saving' } })
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    await started.promise
    expect(name).toBeDisabled()
    expect(screen.getByRole('combobox', { name: '设置分组' })).toBeDisabled()
    const mail = groupButton('邮件发送 有未保存修改')
    expect(mail).toBeDisabled()
    await actor.type(name, ' ignored')
    await actor.click(mail)
    await act(async () => {
      router.history.push('/console/settings?group=mail')
      await Promise.resolve()
    })
    expect(name).toHaveValue('Saving')
    expect(screen.queryByLabelText('邮件密码')).not.toBeInTheDocument()
    pending.resolve()
    await screen.findByText('站点信息已保存。')
    await actor.click(mail)
    expect(screen.getByLabelText('邮件服务器')).toHaveValue('smtp.unsaved.test')
    expect(screen.getByLabelText('邮件密码')).toHaveValue(secret)
  })
  it('uses the response version when saving the next group and keeps saved-secret fields omitted', async () => {
    const bodies: S['SettingsUpdateGroup'][] = []
    server.use(
      http.put('*/api/v1/console/settings/:group', async ({ request, params }) => {
        const body = (await request.json()) as S['SettingsUpdateGroup']
        bodies.push(body)
        return saveResponse(params.group as SettingsGroup, body)
      }),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=site')
    await actor.clear(await screen.findByLabelText('站点名称'))
    await actor.type(screen.getByLabelText('站点名称'), 'Updated platform')
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    await screen.findByText('站点信息已保存。')
    await actor.click(
      within(screen.getByRole('navigation', { name: '设置分组' })).getByRole('button', { name: '邮件发送' }),
    )
    await actor.clear(screen.getByLabelText('邮件服务器'))
    await actor.type(screen.getByLabelText('邮件服务器'), 'smtp.updated.example.test')
    await actor.click(screen.getByRole('button', { name: '保存邮件设置' }))
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[0]?.expected_version).toBe(12)
    expect(bodies[1]?.expected_version).toBe(13)
    expect(bodies[1]?.value).not.toHaveProperty('password')
    expect(JSON.stringify(bodies)).not.toContain('••••••••')
  })

  it.each(['replace', 'clear'] as const)(
    'clears dirty state after %s password and changing host',
    async (operation) => {
      const bodies = recordSaves()
      const actor = userEvent.setup()
      const { router } = mount('/console/settings?group=mail')
      const password = await screen.findByLabelText('邮件密码')
      const replacement = crypto.randomUUID()
      fireEvent.change(screen.getByLabelText('邮件服务器'), { target: { value: 'smtp.changed.test' } })
      if (operation === 'replace') {
        fireEvent.change(password, { target: { value: replacement } })
      } else {
        await actor.click(screen.getByRole('button', { name: '清除邮件密码' }))
        const dialog = screen.getByRole('dialog', { name: '清除邮件密码' })
        await actor.click(within(dialog).getByRole('button', { name: '确认清除' }))
        expect(password).toBeDisabled()
        expect(password).toHaveAttribute('placeholder', '保存后将清除')
        expect(password).toHaveAccessibleDescription('保存后将清除当前邮件密码。')
      }
      await actor.type(screen.getByPlaceholderText('收件人邮箱'), 'ops@example.test')
      const send = screen.getByRole('button', { name: '发送测试邮件' })
      expect(send).toBeDisabled()
      await actor.click(screen.getByRole('button', { name: '保存邮件设置' }))
      await screen.findByText('邮件发送设置已保存。')
      expect(bodies).toMatchObject([
        { value: { host: 'smtp.changed.test', password: operation === 'replace' ? replacement : null } },
      ])
      expect(password).toHaveValue('')
      expect(password).toHaveAccessibleDescription(
        operation === 'replace' ? '已配置；留空会保留当前密钥。' : '尚未配置。',
      )
      expect(screen.queryByText('有未保存修改')).not.toBeInTheDocument()
      expect(send).toBeEnabled()
      await act(async () => {
        await router.navigate({ to: '/' })
      })
      expect(router.state.location.pathname).toBe('/')
      expect(screen.queryByRole('dialog', { name: '离开系统设置？' })).not.toBeInTheDocument()
    },
  )

  it('omits the password from a save after undoing a confirmed clear', async () => {
    const bodies = recordSaves()
    const actor = userEvent.setup()
    mount('/console/settings?group=mail')
    const password = await screen.findByLabelText('邮件密码')
    await actor.click(screen.getByRole('button', { name: '清除邮件密码' }))
    await actor.click(screen.getByRole('button', { name: '确认清除' }))
    await actor.click(screen.getByRole('button', { name: '撤销清除' }))
    expect(password).toBeEnabled()
    expect(password).toHaveAccessibleDescription('已配置；留空会保留当前密钥。')
    fireEvent.change(screen.getByLabelText('邮件服务器'), { target: { value: 'smtp.changed.test' } })
    await actor.click(screen.getByRole('button', { name: '保存邮件设置' }))
    await screen.findByText('邮件发送设置已保存。')
    expect(bodies).toHaveLength(1)
    expect(bodies[0]?.value).not.toHaveProperty('password')
  })

  it('retains and submits domains when the restriction is hidden and shown again', async () => {
    const bodies = recordSaves()
    const actor = userEvent.setup()
    const domains = ['example.test', 'shop.example.test']
    mount('/console/settings?group=security')
    const restriction = await screen.findByRole('checkbox', { name: '限制注册邮箱域名' })
    const domainLabel = '允许的邮箱域名（每行一个）'
    expect(screen.queryByLabelText(domainLabel)).not.toBeInTheDocument()
    await actor.click(restriction)
    fireEvent.change(screen.getByLabelText(domainLabel), { target: { value: domains.join('\n') } })
    await actor.click(restriction)
    expect(screen.queryByLabelText(domainLabel)).not.toBeInTheDocument()
    const save = screen.getByRole('button', { name: '保存安全设置' })
    await actor.click(save)
    await waitFor(() => expect(bodies).toHaveLength(1))
    await waitFor(() => expect(restriction).toBeEnabled())
    expect(bodies[0]?.value).toMatchObject({ restrict_email_domains: false, email_domains: domains })
    await actor.click(restriction)
    expect(screen.getByLabelText(domainLabel)).toHaveValue(domains.join('\n'))
    await actor.click(save)
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[1]?.value).toMatchObject({ restrict_email_domains: true, email_domains: domains })
  })

  it('shows the empty footer state until a link is added', async () => {
    const actor = userEvent.setup()
    mount('/console/settings?group=footer_legal')
    expect(await screen.findByRole('heading', { name: '页脚链接' })).toBeVisible()
    const message = '暂无页脚链接，访客页面底部将不显示链接。'
    expect(screen.getByText(message)).toBeVisible()
    await actor.click(screen.getByRole('button', { name: '新增页脚项目' }))
    expect(screen.queryByText(message)).not.toBeInTheDocument()
    expect(screen.getByLabelText('链接地址')).toBeVisible()
  })

  it('falls back to the site group for an invalid search value', async () => {
    mount('/console/settings?group=unknown')
    expect(await screen.findByRole('heading', { name: /^站点信息$/ })).toBeVisible()
  })

  it('does not submit the previous payment amount when the yuan input is invalid', async () => {
    let saves = 0
    server.use(
      http.put('*/api/v1/console/settings/payment', () => {
        saves += 1
        return success({ version: 13, group: 'payment', value: settings.payment, affected_merchants: 0 })
      }),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=payment')
    await actor.clear(await screen.findByLabelText('最低金额（元）'))
    await actor.type(screen.getByLabelText('最低金额（元）'), '1.234')
    await actor.click(screen.getByRole('button', { name: '保存收款限制' }))
    expect(await screen.findByText('金额范围不符合平台限制。')).toBeVisible()
    expect(saves).toBe(0)
  })

  it('sends mutually exclusive home payload fields for each display mode', async () => {
    const values: Record<string, unknown>[] = []
    server.use(
      http.put('*/api/v1/console/settings/home', async ({ request }) => {
        const body = (await request.json()) as S['SettingsUpdateGroup']
        values.push(body.value)
        return saveResponse('home', body)
      }),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=home')
    await screen.findByLabelText('首页展示方式')
    await actor.click(screen.getByRole('button', { name: '保存首页设置' }))
    await screen.findByText('首页设置已保存。')
    await actor.selectOptions(screen.getByLabelText('首页展示方式'), 'html')
    await actor.type(screen.getByLabelText('HTML'), '<main>首页</main>')
    await actor.click(screen.getByRole('button', { name: '保存首页设置' }))
    await screen.findByText('首页设置已保存。')
    await actor.selectOptions(screen.getByLabelText('首页展示方式'), 'url')
    await actor.type(screen.getByLabelText('首页地址'), 'https://home.example.test')
    await actor.click(screen.getByRole('button', { name: '保存首页设置' }))
    await waitFor(() => expect(values).toHaveLength(3))
    expect(values[0]).toMatchObject({ mode: 'default', html: null, url: null })
    expect(values[1]).toMatchObject({ mode: 'html', html: '<main>首页</main>', url: null })
    expect(values[2]).toMatchObject({ mode: 'url', html: null, url: 'https://home.example.test' })
  })

  it('reports how many merchant settings were adjusted after a payment bounds save', async () => {
    server.use(
      http.put('*/api/v1/console/settings/payment', async ({ request }) => {
        const body = (await request.json()) as S['SettingsUpdateGroup']
        return saveResponse('payment', body, 3)
      }),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=payment')
    await actor.click(await screen.findByRole('button', { name: '保存收款限制' }))
    expect(await screen.findByText('收款限制已保存，3 个商户的收款设置已按新范围调整。')).toBeVisible()
  })

  it('shows a stale billing credential as field errors instead of a version conflict', async () => {
    server.use(
      http.put('*/api/v1/console/settings/billing', () =>
        HttpResponse.json({ error: { code: 'BILLING_CREDENTIAL_STALE', message: 'stale' } }, { status: 409 }),
      ),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=billing')
    await actor.type(await screen.findByLabelText('商户号'), '1234567890123456')
    await actor.click(screen.getByRole('button', { name: '保存订阅收款' }))
    expect(await screen.findAllByText('商户号与密钥不匹配，或该商户已停用/不存在。')).toHaveLength(2)
    expect(screen.queryByLabelText('我已查看最新内容')).not.toBeInTheDocument()
  })

  it('keeps stable public footer order across rename and changes it only on an explicit move', async () => {
    current.footer_legal.items = [
      { ...newFooterItem(), id: 'later', label: { zh_CN: 'Later', en: '' }, sort: 5 },
      { ...newFooterItem(), id: 'first', label: { zh_CN: 'First', en: '' }, sort: 2 },
      { ...newFooterItem(), id: 'second', label: { zh_CN: 'Second', en: '' }, sort: 2 },
    ]
    const bodies = recordSaves()
    const actor = userEvent.setup()
    mount('/console/settings?group=footer_legal')
    const labels = await screen.findAllByLabelText<HTMLInputElement>('中文名称')
    expect(labels.map((input) => input.value)).toEqual(['First', 'Second', 'Later'])
    fireEvent.change(labels[0]!, { target: { value: 'Renamed' } })
    const save = screen.getByRole('button', { name: '保存页脚与协议' })
    await actor.click(save)
    await screen.findByText('页脚与协议已保存。')
    expect(bodies[0]?.value).toMatchObject({
      items: [
        { id: 'first', label: { zh_CN: 'Renamed' }, sort: 0 },
        { id: 'second', sort: 1 },
        { id: 'later', sort: 2 },
      ],
    })
    await actor.click(screen.getAllByRole('button', { name: '上移' })[1]!)
    await actor.click(save)
    await screen.findByText('页脚与协议已保存。')
    expect(bodies[1]?.value).toMatchObject({
      items: [
        { id: 'second', sort: 0 },
        { id: 'first', sort: 1 },
        { id: 'later', sort: 2 },
      ],
    })
  })

  it.each([0, 1])('enforces the actual UTF-8 request limit with %i extra byte', async (extra) => {
    const value = current.footer_legal
    value.terms.zh_CN = '条'.repeat(40000)
    value.terms.en = '款'.repeat(40000)
    value.privacy.en = ''
    const expected_version = current.version
    const bytesLeft = 300 * 1024 - byteLength(JSON.stringify({ expected_version, value }))
    const bodies: string[] = []
    server.use(
      http.put('*/api/v1/console/settings/footer_legal', async ({ request }) => {
        const body = await request.text()
        bodies.push(body)
        return saveResponse('footer_legal', JSON.parse(body) as S['SettingsUpdateGroup'])
      }),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=footer_legal')
    fireEvent.change((await screen.findAllByLabelText('英文内容'))[1]!, {
      target: { value: 'a'.repeat(bytesLeft + extra) },
    })
    await actor.click(screen.getByRole('button', { name: '保存页脚与协议' }))
    if (extra) {
      expect(await screen.findByText(/设置总量超过 300 KB/)).toBeVisible()
      expect(bodies).toHaveLength(0)
    } else {
      await screen.findByText('页脚与协议已保存。')
      expect(bodies).toHaveLength(1)
      expect(byteLength(bodies[0]!)).toBe(300 * 1024)
    }
  })

  it('keeps the send button disabled while the test POST is pending', async () => {
    const pending = deferred<void>()
    const started = deferred<void>()
    let posts = 0
    server.use(
      http.post('*/api/v1/console/settings/mail/test', async () => {
        posts += 1
        started.resolve()
        await pending.promise
        return success({ delivery_id: 'pending', state: 'queued' })
      }),
    )
    const actor = userEvent.setup()
    const view = mount('/console/settings?group=mail')
    await actor.type(await screen.findByPlaceholderText('收件人邮箱'), 'ops@example.test')
    const send = screen.getByRole('button', { name: '发送测试邮件' })
    await actor.click(send)
    await started.promise
    expect(send).toBeDisabled()
    await actor.click(send)
    expect(posts).toBe(1)
    view.unmount()
    pending.resolve()
  })

  it('polls a test email until sent and stops after the terminal state', async () => {
    let polls = 0
    server.use(
      http.post('*/api/v1/console/settings/mail/test', () =>
        success({ delivery_id: 'delivery-1', state: 'queued' }),
      ),
      http.get('*/api/v1/console/settings/mail/test/delivery-1', () => {
        polls += 1
        return success({ id: 'delivery-1', state: 'sent', error_code: null })
      }),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=mail')
    await actor.type(await screen.findByPlaceholderText('收件人邮箱'), 'ops@example.test')
    await actor.click(screen.getByRole('button', { name: '发送测试邮件' }))
    expect(await screen.findByText('测试邮件已发出。', {}, { timeout: 5000 })).toBeVisible()
    expect(polls).toBe(1)
  })

  it('does not poll again after leaving the mail group', async () => {
    let polls = 0
    server.use(
      http.post('*/api/v1/console/settings/mail/test', () =>
        success({ delivery_id: 'delivery-2', state: 'queued' }),
      ),
      http.get('*/api/v1/console/settings/mail/test/delivery-2', () => {
        polls += 1
        return success({ id: 'delivery-2', state: 'sent', error_code: null })
      }),
    )
    const actor = userEvent.setup()
    mount('/console/settings?group=mail')
    await actor.type(await screen.findByPlaceholderText('收件人邮箱'), 'ops@example.test')
    await actor.click(screen.getByRole('button', { name: '发送测试邮件' }))
    await actor.click(
      within(screen.getByRole('navigation', { name: '设置分组' })).getByRole('button', { name: '首页' }),
    )
    await new Promise((resolve) => setTimeout(resolve, 2200))
    expect(polls).toBe(0)
    expect(screen.queryByText('测试邮件已发出。')).not.toBeInTheDocument()
  })
})

describe('settings logo upload', () => {
  const file = () => new File(['fake image content'], 'logo.png', { type: 'image/png' })
  const revoke = vi.fn<(url: string) => void>()
  const decode = vi.fn<() => Promise<void>>()

  beforeEach(() => {
    revoke.mockClear()
    decode.mockReset().mockResolvedValue(undefined)
    vi.stubGlobal(
      'URL',
      class extends URL {
        static createObjectURL = () => 'blob:logo-test'
        static revokeObjectURL = revoke
      },
    )
    vi.stubGlobal('Image', function () {
      return Object.assign(document.createElement('img'), { decode })
    })
    vi.spyOn(HTMLImageElement.prototype, 'naturalWidth', 'get').mockReturnValue(16)
    vi.spyOn(HTMLImageElement.prototype, 'naturalHeight', 'get').mockReturnValue(16)
  })

  it('cancels a pending replacement when removing the logo and saves null', async () => {
    current.site.logo_asset_id = 'existing-logo'
    const pending = deferred<Awaited<ReturnType<typeof consoleApi.uploadSiteImage>>>()
    const upload = vi.spyOn(consoleApi, 'uploadSiteImage').mockReturnValue(pending.promise)
    const bodies = recordSaves()
    const actor = userEvent.setup()
    mount()
    expect(await screen.findByRole('img', { name: '站点标识' })).toHaveAttribute('src', '/media/existing-logo')
    await actor.upload(screen.getByLabelText('站点标识'), file())
    await waitFor(() => expect(upload).toHaveBeenCalledOnce())
    await actor.click(screen.getByRole('button', { name: '移除标识' }))
    expect(upload.mock.calls[0]?.[2]?.aborted).toBe(true)
    expect(screen.queryByRole('img', { name: '站点标识' })).not.toBeInTheDocument()
    await act(async () => {
      pending.resolve({ asset_id: 'obsolete-logo', url: '/media/obsolete-logo' })
      await pending.promise
    })
    expect(screen.queryByRole('img', { name: '站点标识' })).not.toBeInTheDocument()
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    await screen.findByText('站点信息已保存。')
    expect(bodies).toMatchObject([{ value: { logo_asset_id: null } }])
  })

  it('adds only the asset ID when the site name changes during upload', async () => {
    const pending = deferred<Awaited<ReturnType<typeof consoleApi.uploadSiteImage>>>()
    const upload = vi.spyOn(consoleApi, 'uploadSiteImage').mockReturnValue(pending.promise)
    const bodies = recordSaves()
    const actor = userEvent.setup()
    mount()
    await actor.upload(await screen.findByLabelText('站点标识'), file())
    await waitFor(() => expect(upload).toHaveBeenCalledOnce())
    fireEvent.change(screen.getByLabelText('站点名称'), { target: { value: 'Name typed during upload' } })
    await act(async () => {
      pending.resolve({ asset_id: 'logo-1', url: '/media/logo-1' })
      await pending.promise
    })
    expect(screen.getByRole('img', { name: '站点标识' })).toHaveAttribute('src', '/media/logo-1')
    expect(screen.getByLabelText('站点名称')).toHaveValue('Name typed during upload')
    expect(revoke).toHaveBeenCalledExactlyOnceWith('blob:logo-test')
    await actor.click(screen.getByRole('button', { name: '保存站点信息' }))
    await screen.findByText('站点信息已保存。')
    expect(bodies[0]?.value).toMatchObject({ site_name: 'Name typed during upload', logo_asset_id: 'logo-1' })
  })

  it.each(['switch', 'unmount', 'replace'])('discards a late response after %s', async (action) => {
    const pending = deferred<Awaited<ReturnType<typeof consoleApi.uploadSiteImage>>>()
    const upload = vi
      .spyOn(consoleApi, 'uploadSiteImage')
      .mockReturnValueOnce(pending.promise)
      .mockResolvedValue({ asset_id: 'new-logo', url: '/media/new-logo' })
    const actor = userEvent.setup()
    const view = mount()
    await actor.upload(await screen.findByLabelText('站点标识'), file())
    await waitFor(() => expect(upload).toHaveBeenCalledOnce())
    const signal = upload.mock.calls[0]?.[2]
    if (action === 'switch') await actor.click(groupButton('首页'))
    else if (action === 'unmount') view.unmount()
    else await actor.upload(screen.getByLabelText('站点标识'), file())
    expect(signal?.aborted).toBe(true)
    await act(async () => {
      pending.resolve({ asset_id: 'obsolete', url: '/media/obsolete' })
      await pending.promise
    })
    if (action === 'unmount') mount()
    if (action === 'switch') await actor.click(groupButton('站点信息'))
    await screen.findByLabelText('站点名称')
    if (action === 'replace') {
      expect(screen.getByRole('img', { name: '站点标识' })).toHaveAttribute('src', '/media/new-logo')
    } else {
      expect(screen.queryByRole('img', { name: '站点标识' })).not.toBeInTheDocument()
      expect(screen.queryByText('有未保存修改')).not.toBeInTheDocument()
    }
  })

  it.each(['decode', 'dimensions'])('releases the blob without uploading when %s fails', async (failure) => {
    if (failure === 'decode') decode.mockRejectedValueOnce(new Error('invalid image'))
    else vi.spyOn(HTMLImageElement.prototype, 'naturalWidth', 'get').mockReturnValue(4097)
    const upload = vi.spyOn(consoleApi, 'uploadSiteImage')
    const actor = userEvent.setup()
    mount()
    await actor.upload(await screen.findByLabelText('站点标识'), file())
    await screen.findByText(failure === 'decode' ? '标识上传失败，请重试。' : '图片宽高不能超过 4096 像素。')
    expect(revoke).toHaveBeenCalledExactlyOnceWith('blob:logo-test')
    expect(upload).not.toHaveBeenCalled()
  })

  it('releases the blob immediately when unmounting during image decode', async () => {
    const pending = deferred<void>()
    decode.mockReturnValueOnce(pending.promise)
    const upload = vi.spyOn(consoleApi, 'uploadSiteImage')
    const actor = userEvent.setup()
    const view = mount()
    await actor.upload(await screen.findByLabelText('站点标识'), file())
    expect(revoke).not.toHaveBeenCalled()
    view.unmount()
    expect(revoke).toHaveBeenCalledExactlyOnceWith('blob:logo-test')
    await act(async () => {
      pending.resolve()
      await pending.promise
    })
    expect(revoke).toHaveBeenCalledOnce()
    expect(upload).not.toHaveBeenCalled()
  })
})
