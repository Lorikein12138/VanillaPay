import { createMemoryHistory, RouterProvider } from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'
import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest'
import type { components } from '@/api/schema'
import { api, consoleApi } from '@/api/client'
import { AppProviders, createQueryClient } from '@/app/providers'
import { createAppRouter } from '@/app/router'
import { PopupAnnouncements } from '@/features/merchant/NotificationsPage'
import { createI18n } from '@/lib/i18n'
import { publicConfigHandler } from '../public-config-fixture'

type S = components['schemas']
const admin: S['SessionIdentity'] = {
  user: {
    id: '7', role: 'admin', username: 'admin', banned: false, banned_at: null, ban_reason: null,
    totp_enabled: false, locale: 'zh-CN', theme: 'light', created_at: '2026-01-01T00:00:00Z',
  },
  csrf_token: crypto.randomUUID(),
  expires_at: '2030-01-01T00:00:00Z',
}
const announcement: S['PortalAnnouncementDTO'] = {
  id: 'notice-1', revision: 4, popup: true, read: false, published_at: '2026-01-02T00:00:00Z',
  title: { zh_CN: '维护安排', en: '' },
  body: { zh_CN: '服务将于周末维护。', en: '' },
}
const audit: S['PortalAuditDTO'] = {
  id: '1', action: 'announcement.save', actor_role: 'admin', actor_id: '7', merchant_id: '51', ip: null,
  target_type: 'announcement', target_id: 'notice-1', result: 'success', request_id: 'req-1',
  created_at: '2026-01-02T00:00:00Z', safe_metadata: { revision: 4 },
}
const notices = '*/api/v1/console/announcements'
const audits = '*/api/v1/console/audit-logs'
const data = <T,>(value: T, status = 200) => HttpResponse.json({ data: value }, { status })
const page = <T,>(items: T[], next_cursor: string | null = null) => ({ items, next_cursor, total: items.length })
const conflict = () => HttpResponse.json({ error: { code: 'VERSION_CONFLICT' } }, { status: 409 })
let items: S['PortalAnnouncementDTO'][]
const server = setupServer(
  publicConfigHandler,
  http.get('*/api/v1/console/me', () => data(admin)),
  http.get(notices, () => data(page(items))),
)
beforeAll(() => server.listen({ onUnhandledFrame: 'error' }))
beforeEach(() => {
  items = [structuredClone(announcement)]
  api.forgetSession()
  consoleApi.forgetSession()
})
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

function mount(path = '/console/announcements') {
  const router = createAppRouter({ history: createMemoryHistory({ initialEntries: [path] }) })
  render(
    <AppProviders i18n={createI18n()} queryClient={createQueryClient()}>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  return router
}
async function open(action: string, title = action) {
  fireEvent.click((await screen.findAllByRole('button', { name: action }))[0]!)
  return within(await screen.findByRole('dialog', { name: title }))
}
function fill(dialog: Awaited<ReturnType<typeof open>>, label: string, value: string) {
  fireEvent.change(dialog.getByLabelText(label, { exact: false }), { target: { value } })
}

describe('console announcements', () => {
  it('shows the Chinese title and body to English merchants when English is empty', async () => {
    render(
      <AppProviders i18n={createI18n('en')} queryClient={createQueryClient()}>
        <PopupAnnouncements name="merchant" items={[announcement]} />
      </AppProviders>,
    )
    const dialog = await screen.findByRole('dialog', { name: '维护安排' })
    expect(within(dialog).getByText('服务将于周末维护。')).toBeVisible()
  })

  it('publishes with optional English and previews literal text, then refreshes the list', async () => {
    items = []
    const bodies: unknown[] = []
    server.use(http.post(notices, async ({ request }) => {
      bodies.push(await request.json())
      items = [{ ...announcement, title: { zh_CN: '平台维护', en: '' } }]
      return data(items[0], 201)
    }))
    mount()
    expect(await screen.findByText('还没有公告')).toBeVisible()
    const dialog = await open('新建公告')
    const publish = dialog.getByRole('button', { name: '发布公告' })
    expect(publish).toBeDisabled()
    fill(dialog, '中文标题', '平台维护')
    const body = '<b>按原样显示</b>\n**不要渲染**'
    fill(dialog, '中文正文', body)
    fireEvent.click(dialog.getByRole('button', { name: '正文预览' }))
    expect(dialog.getByText(body, { selector: 'dd', normalizer: (text) => text })).toBeVisible()
    expect(dialog.queryByText('按原样显示', { exact: true })).not.toBeInTheDocument()
    fireEvent.click(dialog.getByRole('checkbox', { name: '登录后弹窗提醒' }))
    fireEvent.click(publish)
    expect(await screen.findByText('公告已发布。')).toBeVisible()
    expect(bodies).toEqual([{ title: { zh_CN: '平台维护', en: '' }, body: { zh_CN: body, en: '' }, popup: true }])
    expect(within(screen.getByRole('table', { name: '公告' })).getByText('平台维护')).toBeVisible()
  })

  it('checks UTF-8 byte limits on each field and allows the exact limits', async () => {
    mount()
    const dialog = await open('新建公告')
    const publish = dialog.getByRole('button', { name: '发布公告' })
    fill(dialog, '中文标题', '中'.repeat(101))
    fill(dialog, '英文标题', 'a'.repeat(301))
    fill(dialog, '中文正文', '中'.repeat(33334))
    fill(dialog, '英文正文', '😀'.repeat(25001))
    for (const [label, size] of [['中文标题', 300], ['英文标题', 300], ['中文正文', 100000], ['英文正文', 100000]] as const) {
      expect(dialog.getByLabelText(label, { exact: false })).toHaveAccessibleDescription(expect.stringContaining(`${size} 字节`))
    }
    expect(publish).toBeDisabled()
    fill(dialog, '中文标题', '中'.repeat(100))
    fill(dialog, '英文标题', 'a'.repeat(300))
    fill(dialog, '中文正文', '中'.repeat(33333) + 'a')
    fill(dialog, '英文正文', '😀'.repeat(25000))
    expect(publish).toBeEnabled()
  })

  it('refreshes on conflict and saves only the explicitly reviewed latest revision', async () => {
    const bodies: unknown[] = []
    server.use(http.put(notices + '/notice-1', async ({ request }) => {
      const body = await request.json() as S['PortalAnnouncementCommand']
      bodies.push(body)
      if (bodies.length === 1) {
        items = [{ ...announcement, revision: 5, title: { zh_CN: '另一位管理员的标题', en: '' } }]
        return conflict()
      }
      items = [{ ...announcement, title: body.title, revision: 6 }]
      return data(items[0])
    }))
    mount()
    const dialog = await open('编辑', '编辑公告')
    expect(dialog.getByText(/商户将再次看到弹窗/)).toBeVisible()
    fill(dialog, '中文标题', '未保存的旧草稿')
    const save = dialog.getByRole('button', { name: '保存修改' })
    fireEvent.click(save)
    expect(await dialog.findByText('公告已被其他管理员修改，已刷新为最新内容')).toBeVisible()
    expect(dialog.getByLabelText('中文标题')).toHaveValue('另一位管理员的标题')
    expect(save).toBeDisabled()
    fireEvent.click(dialog.getByRole('checkbox', { name: '我已查看最新内容' }))
    fill(dialog, '中文标题', '最终标题')
    fireEvent.click(save)
    expect(await screen.findByText('公告已保存。')).toBeVisible()
    expect(bodies).toEqual([
      { title: { zh_CN: '未保存的旧草稿', en: '' }, body: announcement.body, popup: true, revision: 4 },
      { title: { zh_CN: '最终标题', en: '' }, body: announcement.body, popup: true, revision: 5 },
    ])
  })

  it('sends only the revision to delete and refreshes the empty state', async () => {
    items[0]!.popup = false
    const bodies: unknown[] = []
    server.use(http.delete(notices + '/notice-1', async ({ request }) => {
      bodies.push(await request.json())
      items = []
      return data(null)
    }))
    mount()
    const editor = await open('编辑', '编辑公告')
    expect(editor.queryByText(/商户将再次看到弹窗/)).not.toBeInTheDocument()
    fireEvent.click(editor.getByRole('button', { name: '取消' }))
    const dialog = await open('删除', '删除公告')
    expect(dialog.getByText('删除后，商户端将不再显示此公告。')).toBeVisible()
    fireEvent.click(dialog.getByRole('button', { name: '删除' }))
    expect(await screen.findByText('公告已删除。')).toBeVisible()
    expect(await screen.findByText('还没有公告')).toBeVisible()
    expect(bodies).toEqual([{ revision: 4 }])
  })

  it.each([
    ['Chinese body', { body: { ...announcement.body, zh_CN: '<b>维护改为周一</b>\n请留意通知。' } }],
    ['English text', { title: { ...announcement.title, en: 'Maintenance' }, body: { ...announcement.body, en: 'New schedule' } }],
    ['popup status', { popup: false }],
  ] as const)('requires reviewing the new delete revision when only the %s changes', async (_field, changes) => {
    const latest = { ...announcement, ...changes, revision: 5 }
    const newest = { ...latest, revision: 6, body: { ...latest.body, zh_CN: '再次变更的正文' } }
    const bodies: unknown[] = []
    server.use(http.delete(notices + '/notice-1', async ({ request }) => {
      bodies.push(await request.json())
      if (bodies.length <= 2) {
        items = [bodies.length === 1 ? latest : newest]
        return conflict()
      }
      items = []
      return data(null)
    }))
    mount()
    const dialog = await open('删除', '删除公告')
    const remove = dialog.getByRole<HTMLButtonElement>('button', { name: '删除' })
    const review = () => dialog.getByRole('checkbox', { name: '我已查看最新内容' })
    const detail = (value: string) => dialog.getByText(value, { selector: 'dd', normalizer: (text) => text })
    function verifyBlocked() {
      expect(review()).not.toBeChecked()
      expect(remove).toBeDisabled()
      fireEvent.click(remove)
      fireEvent.submit(remove.form!)
    }
    fireEvent.click(remove)
    await dialog.findByText('公告已被其他管理员修改，已刷新为最新内容')
    for (const value of [latest.title.zh_CN, latest.body.zh_CN, latest.popup ? '已开启' : '未开启', '5']) {
      expect(detail(value)).toBeVisible()
    }
    if (latest.title.en) expect(detail(latest.title.en)).toBeVisible()
    if (latest.body.en) expect(detail(latest.body.en)).toBeVisible()
    expect(dialog.queryByText('维护改为周一', { exact: true })).not.toBeInTheDocument()
    verifyBlocked()
    expect(bodies).toEqual([{ revision: 4 }])
    fireEvent.click(review())
    fireEvent.click(remove)
    expect(await dialog.findByText('再次变更的正文', { selector: 'dd' })).toBeVisible()
    expect(detail('6')).toBeVisible()
    verifyBlocked()
    expect(bodies).toEqual([{ revision: 4 }, { revision: 5 }])
    fireEvent.click(review())
    fireEvent.click(remove)
    expect(await screen.findByText('公告已删除。')).toBeVisible()
    expect(bodies).toEqual([{ revision: 4 }, { revision: 5 }, { revision: 6 }])
  })

  it('keeps a stale draft blocked until a failed refresh is retried across pages', async () => {
    let phase: 'initial' | 'failed' | 'retry' = 'initial'
    const latest = { ...announcement, revision: 5, title: { zh_CN: '最新标题', en: '' } }
    server.use(
      http.put(notices + '/notice-1', () => {
        phase = 'failed'
        return conflict()
      }),
      http.get(notices, ({ request }) => {
        if (phase === 'initial') return data(page(items))
        if (phase === 'failed') {
          phase = 'retry'
          return HttpResponse.json({ error: { code: 'INTERNAL_ERROR' } }, { status: 500 })
        }
        if (new URL(request.url).searchParams.get('cursor') === 'older') return data(page([latest]))
        return data(page([{ ...announcement, id: 'notice-new' }], 'older'))
      }),
    )
    mount()
    const dialog = await open('编辑', '编辑公告')
    fill(dialog, '中文标题', '尚未保存')
    const save = dialog.getByRole('button', { name: '保存修改' })
    fireEvent.click(save)
    await dialog.findByRole('alert')
    expect(dialog.getByLabelText('中文标题')).toHaveValue('尚未保存')
    expect(save).toBeDisabled()
    fireEvent.click(dialog.getByRole('button', { name: '重试' }))
    expect(await dialog.findByText('公告已被其他管理员修改，已刷新为最新内容')).toBeVisible()
    expect(dialog.getByLabelText('中文标题')).toHaveValue('最新标题')
    expect(save).toBeDisabled()
  })

  it('uses the server cursor to load more announcements', async () => {
    const cursors: Array<string | null> = []
    server.use(http.get(notices, ({ request }) => {
      const cursor = new URL(request.url).searchParams.get('cursor')
      cursors.push(cursor)
      return data(cursor ? page([{ ...announcement, id: 'notice-2', title: { zh_CN: '历史公告', en: '' } }]) : page(items, 'older'))
    }))
    mount()
    fireEvent.click(await screen.findByRole('button', { name: '加载更多' }))
    expect(await within(screen.getByRole('table', { name: '公告' })).findByText('历史公告')).toBeVisible()
    expect(cursors).toEqual([null, 'older'])
  })
})

describe('role-aware audit log', () => {
  it.each([
    ['console', 'start', '2026-11-08'],
    ['console', 'end', '2026-02-29T12:00:00Z'],
    ['merchant', 'start', 'not-a-date'],
    ['merchant', 'end', '2026-11-08T12:00:00'],
  ] as const)('drops an invalid %s audit %s (%s) on the first request and starts new filters from page one', async (role, key, value) => {
    const isConsole = role === 'console'
    const requests: URLSearchParams[] = []
    const validKey = key === 'start' ? 'end' : 'start'
    const valid = '2024-02-29T12:00:00.123456789+08:00'
    server.use(
      http.get('*/api/v1/merchant/me', () => data({ ...admin, user: { ...admin.user, role: 'merchant', pid: '100051' } })),
      http.get('*/api/v1/merchant/notifications', () => data({ items: [], popup_announcements: [], unread_count: 0 })),
      http.get(`*/api/v1/${role}/audit-logs`, ({ request }) => {
        const params = new URL(request.url).searchParams
        requests.push(params)
        return data(params.has('cursor') ? page([{ ...audit, id: '2' }]) : page([audit], 'older'))
      }),
    )
    const search = new URLSearchParams({ [key]: value, [validKey]: valid, action: 'announcement.save' })
    if (isConsole) search.set('actor_role', 'admin')
    mount(`${isConsole ? '/console/audit-logs' : '/account/logs'}?${search}`)
    const table = await screen.findByRole('table', { name: isConsole ? '审计日志' : '操作日志' })
    expect(within(table).getByRole('button', { name: '详情' })).toBeVisible()
    expect(Object.fromEntries(requests[0]!)).toEqual({
      [validKey]: valid, action: 'announcement.save', limit: '20', ...(isConsole && { actor_role: 'admin' }),
    })
    fireEvent.click(screen.getByRole('button', { name: '加载更多' }))
    await waitFor(() => expect(within(table).getAllByRole('row')).toHaveLength(3))
    expect(requests[1]?.get('cursor')).toBe('older')
    fireEvent.change(screen.getByLabelText('操作类型'), { target: { value: 'auth.login' } })
    fireEvent.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(requests[2]?.get('action')).toBe('auth.login'))
    expect(requests[2]?.has('cursor')).toBe(false)
    expect(requests[2]?.has(key)).toBe(false)
  })

  it('adds merchant links and console filters, sends the exact ID, and paginates', async () => {
    const requests: URLSearchParams[] = []
    server.use(http.get(audits, ({ request }) => {
      const params = new URL(request.url).searchParams
      requests.push(params)
      return data(params.get('cursor') ? page([{ ...audit, id: '2', merchant_id: null }]) : page([audit], 'older'))
    }))
    const router = mount('/console/audit-logs?merchant_id=9223372036854775807&actor_role=admin')
    const table = await screen.findByRole('table', { name: '审计日志' })
    expect(within(table).getByRole('columnheader', { name: '商户' })).toBeVisible()
    expect(within(table).getByRole('link', { name: '51' })).toHaveAttribute('href', '/console/merchants/51')
    expect(screen.getByLabelText('商户 ID')).toHaveValue('9223372036854775807')
    expect(screen.getByLabelText('操作者角色')).toHaveValue('admin')
    fireEvent.click(within(table).getByRole('button', { name: '详情' }))
    const dialog = await screen.findByRole('dialog', { name: '操作详情' })
    expect(within(dialog).getByText(/管理员 ·/)).toHaveTextContent('管理员 · 7')
    fireEvent.click(within(dialog).getByRole('button', { name: '关闭' }))
    fireEvent.click(screen.getByRole('button', { name: '加载更多' }))
    await waitFor(() => expect(within(table).getAllByRole('row')).toHaveLength(3))
    expect(requests[1]?.get('cursor')).toBe('older')
    fireEvent.change(screen.getByLabelText('商户 ID'), { target: { value: '99' } })
    fireEvent.change(screen.getByLabelText('操作者角色'), { target: { value: 'device' } })
    fireEvent.click(screen.getByRole('button', { name: '应用筛选' }))
    await waitFor(() => expect(requests).toHaveLength(3))
    expect(Object.fromEntries(requests[0]!)).toMatchObject({ merchant_id: '9223372036854775807', actor_role: 'admin' })
    expect(Object.fromEntries(requests[2]!)).toEqual({ merchant_id: '99', actor_role: 'device', limit: '20' })
    expect(router.state.location.search).toMatchObject({ merchant_id: 99, actor_role: 'device' })
  })

  it.each(['0', '-51', '1.5', '1e3', '9223372036854775808'])('drops an invalid merchant ID %s and actor role at the URL boundary', async (id) => {
    const requests: URLSearchParams[] = []
    server.use(http.get(audits, ({ request }) => {
      requests.push(new URL(request.url).searchParams)
      return data(page([]))
    }))
    mount(`/console/audit-logs?merchant_id=${id}&actor_role=guest&action=announcement.save`)
    expect(await screen.findByText('暂无符合条件的操作记录')).toBeVisible()
    expect(Object.fromEntries(requests[0]!)).toEqual({ action: 'announcement.save', limit: '20' })
  })

  it('keeps merchant filters and actor detail unchanged', async () => {
    const requests: URLSearchParams[] = []
    server.use(
      http.get('*/api/v1/merchant/me', () => data({ ...admin, user: { ...admin.user, role: 'merchant', pid: '100051' } })),
      http.get('*/api/v1/merchant/notifications', () => data({ items: [], popup_announcements: [], unread_count: 0 })),
      http.get('*/api/v1/merchant/audit-logs', ({ request }) => {
        requests.push(new URL(request.url).searchParams)
        return data(page([{ ...audit, actor_role: 'merchant', actor_id: '51' }]))
      }),
    )
    mount('/account/logs?merchant_id=51&actor_role=admin&result=success')
    const table = await screen.findByRole('table', { name: '操作日志' })
    expect(within(table).queryByRole('columnheader', { name: '商户' })).not.toBeInTheDocument()
    expect(screen.queryByLabelText('商户 ID')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('操作者角色')).not.toBeInTheDocument()
    expect(Object.fromEntries(requests[0]!)).toEqual({ result: 'success', limit: '20' })
    fireEvent.click(within(table).getByRole('button', { name: '详情' }))
    const dialog = await screen.findByRole('dialog', { name: '操作详情' })
    expect(within(dialog).getByText('商户', { exact: true })).toBeVisible()
    expect(within(dialog).queryByText('51', { exact: true })).not.toBeInTheDocument()
  })
})
