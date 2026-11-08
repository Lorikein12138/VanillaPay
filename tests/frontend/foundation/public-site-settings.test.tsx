import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { StrictMode } from 'react'
import { afterAll, afterEach, beforeAll, expect, it, vi } from 'vitest'
import type { components } from '@/api/schema'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { createI18n, type Language } from '@/lib/i18n'
import { defaultPublicConfig, publicConfigHandler } from '../public-config-fixture'

type Config = components['schemas']['PublicConfig']
const endpoint = '*/api/v1/public/config'
const server = setupServer(publicConfigHandler)
const defaultTitle = '用自己的收款码，收得清清楚楚'
const sandbox = 'allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation'
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
afterEach(() => {
  server.resetHandlers()
  vi.unstubAllGlobals()
})
afterAll(() => server.close())

function mount(path = '/', language: Language = 'zh-CN') {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  render(
    <StrictMode>
      <AppProviders i18n={createI18n(language)} queryClient={createQueryClient()}>
        <RouterProvider router={router} />
      </AppProviders>
    </StrictMode>,
  )
}

function mockConfig(config: Partial<Config>) {
  server.use(http.get(endpoint, () => HttpResponse.json({ data: { ...defaultPublicConfig, ...config } })))
}

it('uses one config request for the home brand, both logos, footer, and document title', async () => {
  let requests = 0
  server.use(http.get(endpoint, () => {
    requests += 1
    return HttpResponse.json({ data: { ...defaultPublicConfig, site_name: '星河支付', logo_url: '/media/test-logo' } })
  }))
  mount()
  const brand = await screen.findByRole('link', { name: '星河支付 首页' })
  const logo = within(brand).getByRole('img', { name: '星河支付' })
  expect(logo).toHaveAttribute('src', '/media/test-logo')
  expect(logo).toHaveClass('size-7', 'object-contain')
  expect(within(screen.getByRole('contentinfo')).getByRole('img', { name: '星河支付' })).toHaveClass('size-5')
  expect(document.title).toBe('星河支付')
  expect(requests).toBe(1)
  fireEvent.error(logo)
  expect(within(brand).queryByRole('img')).not.toBeInTheDocument()
  expect(brand.querySelector('svg')).toBeInTheDocument()
})

it('falls back to the default name and mark when settings are empty', async () => {
  mockConfig({ site_name: '  ', logo_url: '', footer: null })
  mount()
  const brand = await screen.findByRole('link', { name: 'VanillaPay 首页' })
  expect(brand.querySelector('svg')).toBeInTheDocument()
  expect(within(brand).queryByRole('img')).not.toBeInTheDocument()
  const footer = within(screen.getByRole('contentinfo'))
  expect(footer.getByText('VanillaPay')).toBeVisible()
  expect(footer.queryByRole('list')).not.toBeInTheDocument()
  expect(footer.getAllByRole('link').map((link) => link.getAttribute('href'))).toEqual(['/legal/terms', '/legal/privacy'])
  expect(document.title).toBe('VanillaPay')
})

it('renders translated footer labels, descriptions and safe links in backend order', async () => {
  const item = (id: string, zh_CN: string, en: string, url: string | null, text = '') => ({
    id, label: { zh_CN, en }, url, text, sort: 0, enabled: true,
  })
  mockConfig({ footer: [
    item('help', '帮助中心', '', 'https://example.test/help', '<b>Service hours</b>'),
    item('status', '平台状态', 'Service status', 'http://example.test/status'),
    item('mail', '联系我们', 'Email support', 'mailto:help@example.test'),
    item('notice', '维护安排', 'Maintenance', null, 'Sunday mornings'),
    item('unsafe', '无效链接', 'Invalid link', 'javascript:alert(1)'),
  ] })
  mount('/', 'en')
  const footer = within(await screen.findByRole('contentinfo'))
  expect(footer.getAllByRole('listitem').map((entry) => entry.textContent)).toEqual([
    '帮助中心<b>Service hours</b>', 'Service status', 'Email support', 'MaintenanceSunday mornings', 'Invalid link',
  ])
  for (const [label, href] of [['帮助中心', 'https://example.test/help'], ['Service status', 'http://example.test/status']] as const) {
    const link = footer.getByRole('link', { name: label })
    expect(link).toHaveAttribute('href', href)
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  }
  const mail = footer.getByRole('link', { name: 'Email support' })
  expect(mail).toHaveAttribute('href', 'mailto:help@example.test')
  expect(mail).not.toHaveAttribute('target')
  expect(mail).not.toHaveAttribute('rel')
  expect(footer.queryByRole('link', { name: /Maintenance|Invalid link/ })).not.toBeInTheDocument()
  expect(footer.getByText('<b>Service hours</b>')).toBeVisible()
})

it('shows only a loading state until config resolves, then displays HTML in the exact sandbox', async () => {
  const html = '<h1 id="custom-home">Custom home</h1><script>void 0</script>'
  let release!: () => void
  const pending = new Promise<void>((resolve) => { release = resolve })
  server.use(http.get(endpoint, async () => {
    await pending
    return HttpResponse.json({ data: {
      ...defaultPublicConfig, site_name: '星河支付', home: { mode: 'html', html, url: null },
    } })
  }))
  mount()
  try {
    expect(await screen.findByRole('status')).toBeVisible()
    expect(screen.queryByText(defaultTitle)).not.toBeInTheDocument()
    expect(screen.queryByRole('banner')).not.toBeInTheDocument()
    expect(screen.queryByRole('contentinfo')).not.toBeInTheDocument()
  } finally { release() }
  const frame = await screen.findByTitle('星河支付')
  expect(frame.tagName).toBe('IFRAME')
  expect(frame).toHaveAttribute('srcdoc', html)
  expect(frame).toHaveAttribute('sandbox', sandbox)
  expect(document.querySelector('#custom-home')).not.toBeInTheDocument()
  expect(screen.queryByRole('banner')).not.toBeInTheDocument()
  expect(screen.queryByRole('contentinfo')).not.toBeInTheDocument()
})

it('replaces the location once and offers a manual link for an HTTPS homepage', async () => {
  const destination = 'https://example.test/landing?lang=en'
  mockConfig({ home: { mode: 'url', html: null, url: destination } })
  const replace = vi.fn()
  vi.stubGlobal('location', { ...location, replace })
  mount()
  expect(await screen.findByRole('link', { name: '如未自动跳转，请点击继续' })).toHaveAttribute('href', destination)
  expect(screen.getByRole('status')).toHaveTextContent('正在跳转')
  await waitFor(() => expect(replace).toHaveBeenCalledExactlyOnceWith(destination))
  expect(screen.queryByText(defaultTitle)).not.toBeInTheDocument()
})

it.each(['http://example.test/', 'javascript:alert(1)', '//example.test/', 'https://'])('shows the default home for %s', async (url) => {
  mockConfig({ home: { mode: 'url', html: null, url } })
  mount()
  expect(await screen.findByRole('heading', { name: defaultTitle })).toBeVisible()
  expect(screen.queryByRole('link', { name: '如未自动跳转，请点击继续' })).not.toBeInTheDocument()
})

it('shows the default home and footer when config fails', async () => {
  server.use(http.get(endpoint, () => new HttpResponse(null, { status: 503 })))
  mount()
  expect(await screen.findByRole('heading', { name: defaultTitle })).toBeVisible()
  expect(within(screen.getByRole('contentinfo')).getByText('VanillaPay')).toBeVisible()
})
