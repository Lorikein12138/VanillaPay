import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it } from 'vitest'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { createI18n } from '@/lib/i18n'
import { initTheme } from '@/lib/theme'

const zhTitle = '用自己的收款码，收得清清楚楚'
const enTitle = 'Collect with your own payment codes, and know exactly what arrived'

function renderApp(path = '/') {
  initTheme()
  const i18n = createI18n()
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  const view = render(
    <AppProviders i18n={i18n} queryClient={createQueryClient()}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  return { router, view }
}

describe('foundation bootstrap', () => {
  it('renders the lazy home page in zh-CN with language and theme controls', async () => {
    renderApp('/')

    expect(await screen.findByRole('heading', { level: 1, name: zhTitle })).toBeVisible()
    expect(document.documentElement.lang).toBe('zh-CN')
    expect(screen.getByRole('button', { name: '切换语言为 English' })).toBeVisible()

    const theme = screen.getByRole('group', { name: '主题' })
    expect(within(theme).getByRole('button', { name: '跟随系统' })).toHaveAttribute('aria-pressed', 'true')
    expect(within(theme).getByRole('button', { name: '浅色' })).toHaveAttribute('aria-pressed', 'false')
    expect(within(theme).getByRole('button', { name: '深色' })).toHaveAttribute('aria-pressed', 'false')
    expect(document.documentElement).not.toHaveAttribute('data-theme')
  })

  it('switches language in place and persists it for the next visit', async () => {
    const user = userEvent.setup()
    const { router, view } = renderApp('/')
    await screen.findByRole('heading', { level: 1, name: zhTitle })

    await user.click(screen.getByRole('button', { name: '切换语言为 English' }))

    expect(screen.getByRole('heading', { level: 1, name: enTitle })).toBeVisible()
    expect(screen.getByRole('group', { name: 'Theme' })).toBeVisible()
    expect(document.documentElement.lang).toBe('en')
    expect(localStorage.getItem('vp-language')).toBe('en')
    expect(router.state.location.pathname).toBe('/')

    view.unmount()
    renderApp('/')
    expect(await screen.findByRole('heading', { level: 1, name: enTitle })).toBeVisible()
    expect(screen.getByRole('button', { name: 'Switch language to 中文' })).toBeVisible()
    expect(document.documentElement.lang).toBe('en')
  })

  it('applies and persists the three theme modes', async () => {
    const user = userEvent.setup()
    renderApp('/')
    await screen.findByRole('heading', { level: 1, name: zhTitle })
    const theme = screen.getByRole('group', { name: '主题' })

    await user.click(within(theme).getByRole('button', { name: '深色' }))
    expect(document.documentElement).toHaveAttribute('data-theme', 'dark')
    expect(localStorage.getItem('vp-theme')).toBe('dark')
    expect(within(theme).getByRole('button', { name: '深色' })).toHaveAttribute('aria-pressed', 'true')

    await user.click(within(theme).getByRole('button', { name: '浅色' }))
    expect(document.documentElement).toHaveAttribute('data-theme', 'light')
    expect(localStorage.getItem('vp-theme')).toBe('light')

    await user.click(within(theme).getByRole('button', { name: '跟随系统' }))
    expect(document.documentElement).not.toHaveAttribute('data-theme')
    expect(localStorage.getItem('vp-theme')).toBe('system')
  })

  it('shows the 404 UI for unknown paths without changing the URL', async () => {
    const user = userEvent.setup()
    const { router } = renderApp('/no-such-page')

    expect(await screen.findByRole('heading', { level: 1, name: '页面不存在' })).toBeVisible()
    expect(router.state.location.pathname).toBe('/no-such-page')
    expect(screen.getByRole('button', { name: '切换语言为 English' })).toBeVisible()
    expect(screen.getByRole('group', { name: '主题' })).toBeVisible()

    await user.click(screen.getByRole('link', { name: '返回首页' }))
    expect(await screen.findByRole('heading', { level: 1, name: zhTitle })).toBeVisible()
    expect(router.state.location.pathname).toBe('/')
  })
})
