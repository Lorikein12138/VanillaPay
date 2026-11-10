import { expect, test, type APIRequestContext, type Page } from '@playwright/test'
import type { components, operations } from '../../../web/src/api/schema'
import { clickControl, login, polishShot } from '../merchant/fixtures'

type Account = Parameters<typeof login>[1]
type Admin = Omit<Account, 'pid'>
type Helpers = {
  seed: (request: APIRequestContext) => Promise<Admin>
  captcha: (page: Page, token: string) => Promise<void>
  signIn: (page: Page, admin: Admin) => Promise<void>
}
type Settings = components['schemas']['Settings']
type LogoUpload = NonNullable<operations['post_api_v1_console_assets_logo']['responses'][201]['content']['application/json']['data']>
type Group = 'site' | 'home' | 'footer_legal'
const settingsURL = '/api/v1/console/settings'
const name = '星河支付'
const destination = 'https://public-site.example.test/welcome'
const html = `<!doctype html>
<html lang="zh-CN">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
  :root { color-scheme: light dark; font: 18px/1.6 system-ui, sans-serif; }
  body { min-height: 100vh; display: grid; place-items: center; margin: 0; }
  main { max-width: 32rem; padding: 2rem; }
  small { color: light-dark(#8a453a, #f4aa99); }
  h1 { font-size: clamp(2rem, 7vw, 4rem); margin: .5rem 0; }
</style>
<main><small>星河支付 · 平台服务</small><h1>欢迎来到星河支付</h1><p>在一个地方查看支付进度，管理你的收款。</p></main>
<output id="parent-check" hidden></output><output id="storage-check" hidden></output>
<script>
  function probe(id, read) {
    let result = 'allowed';
    try { read(); } catch (error) { result = error.name; }
    document.getElementById(id).textContent = result;
  }
  probe('parent-check', () => parent.document.title);
  probe('storage-check', () => localStorage.getItem('vp-theme'));
</script></html>`

async function snapshot(page: Page, state: string, fullPage: boolean) {
  await polishShot(page, state, fullPage, 'public-site-settings-shots', 'assert-empty')
}

export function registerPublicSiteTests({ seed, captcha, signIn }: Helpers) {
  test('publishes site branding and footer and exercises all real homepage modes', {
    tag: '@public-site-settings',
  }, async ({ page, request, browser }) => {
    test.setTimeout(150_000)
    const admin = await seed(request)
    await captcha(page, admin.turnstileToken)
    await signIn(page, admin)
    await expect(page).toHaveURL(/\/console$/)
    const origin = new URL(page.url()).origin
    const identity = await page.request.get('/api/v1/console/me')
    expect(identity.ok()).toBe(true)
    const { data: { csrf_token } } = await identity.json() as { data: components['schemas']['SessionIdentity'] }
    const headers = { Origin: origin, 'X-CSRF-Token': csrf_token }
    async function readSettings() {
      const response = await page.request.get(settingsURL)
      expect(response.ok()).toBe(true)
      return (await response.json() as { data: Settings }).data
    }
    const original = await readSettings()
    const modified = new Set<Group>()
    async function save<K extends Group>(group: K, value: Settings[K]) {
      const { version } = await readSettings()
      modified.add(group)
      const response = await page.request.put(`${settingsURL}/${group}`, { headers, data: { expected_version: version, value } })
      expect(response.status(), `save ${group}`).toBe(200)
    }
    async function restore() {
      const errors: unknown[] = []
      for (const group of Array.from(modified).reverse()) {
        try { await save(group, original[group]) } catch (error) { errors.push(error) }
      }
      if (errors.length) throw new AggregateError(errors, 'restore all touched public settings')
      const restored = await readSettings()
      for (const group of modified) expect(restored[group], `restore ${group}`).toEqual(original[group])
    }

    const merchantResponse = await request.post('/__e2e/merchant', { data: {} })
    expect(merchantResponse.ok()).toBe(true)
    const merchant = await merchantResponse.json() as Account
    const visitorContext = await browser.newContext({ baseURL: origin, viewport: { width: 1280, height: 800 } })
    const visitor = await visitorContext.newPage()
    await captcha(visitor, merchant.turnstileToken)
    try {
      const png = await page.evaluate(() => {
        const canvas = document.createElement('canvas')
        canvas.width = canvas.height = 56
        const context = canvas.getContext('2d')!
        context.fillStyle = '#294b61'
        context.roundRect(0, 0, 56, 56, 14)
        context.fill()
        context.fillStyle = '#ffcf9f'
        context.beginPath()
        context.arc(28, 22, 10, 0, Math.PI * 2)
        context.fill()
        context.fillRect(14, 35, 28, 4)
        return canvas.toDataURL('image/png').split(',')[1]!
      })
      const upload = await page.request.post('/api/v1/console/assets/logo', {
        headers, multipart: { file: { name: 'platform.png', mimeType: 'image/png', buffer: Buffer.from(png, 'base64') } },
      })
      expect(upload.status()).toBe(201)
      const { data: { asset_id } } = await upload.json() as { data: LogoUpload }
      const logo = `/media/${asset_id}`
      await save('site', { site_name: name, logo_asset_id: asset_id, favicon_asset_id: original.site.favicon_asset_id })
      await save('footer_legal', {
        ...original.footer_legal,
        items: [
          { id: 'help', label: { zh_CN: '帮助中心', en: 'Help center' }, text: '查看收款与接入指南。',
            url: 'https://help.example.test/', enabled: true, sort: 0 },
          { id: 'support', label: { zh_CN: '服务时间', en: 'Support hours' }, text: '工作日 09:00–18:00',
            url: null, enabled: true, sort: 1 },
          { id: 'hidden', label: { zh_CN: '隐藏项目', en: 'Hidden item' }, text: '', url: null, enabled: false, sort: 2 },
        ],
      })
      await save('home', { mode: 'default', html: null, url: null })

      async function checkPublicFooter() {
        const footer = visitor.getByRole('contentinfo')
        await expect(footer.getByRole('img', { name })).toHaveAttribute('src', logo)
        await expect(footer.getByRole('link', { name: '帮助中心' })).toHaveAttribute('href', 'https://help.example.test/')
        await expect(footer.getByText('服务时间')).toBeVisible()
        await expect(footer.getByText('隐藏项目')).toHaveCount(0)
        await expect(footer.getByRole('link', { name: '服务条款' })).toHaveAttribute('href', '/legal/terms')
        await expect(footer.getByRole('link', { name: '隐私政策' })).toHaveAttribute('href', '/legal/privacy')
      }

      await visitor.goto('/')
      const brand = visitor.getByRole('banner').getByRole('link', { name: `${name} 首页` })
      const brandLogo = brand.getByRole('img', { name })
      await expect(brandLogo).toHaveAttribute('src', logo)
      await expect.poll(() => brandLogo.evaluate((image: HTMLImageElement) => image.naturalWidth)).toBeGreaterThan(0)
      await expect(visitor).toHaveTitle(name)
      await checkPublicFooter()
      for (const width of [360, 1280]) {
        await visitor.setViewportSize({ width, height: width === 360 ? 780 : 800 })
        for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
          await visitor.goto('/')
          await visitor.getByRole('button', { name: label, exact: true }).click()
          await expect(visitor.locator('html')).toHaveAttribute('data-theme', theme)
          await snapshot(visitor, 'home-default', true)
          await visitor.goto('/login')
          await checkPublicFooter()
          await snapshot(visitor, 'login', true)
        }
      }

      await login(visitor, merchant)
      await expect(visitor).toHaveURL(/\/dashboard$/)
      for (const width of [360, 1280]) {
        await visitor.setViewportSize({ width, height: width === 360 ? 780 : 800 })
        const area = visitor.getByRole(width === 360 ? 'banner' : 'complementary')
        await expect(area.getByRole('link', { name, exact: true }).getByRole('img', { name })).toHaveAttribute('src', logo)
        for (const [theme, label] of [['light', '浅色'], ['dark', '深色']] as const) {
          const saved = visitor.waitForResponse((response) =>
            response.url().endsWith('/merchant/preferences') && response.request().method() === 'PATCH',
          )
          await clickControl(visitor, label)
          expect((await saved).ok()).toBe(true)
          await expect(visitor.locator('html')).toHaveAttribute('data-theme', theme)
          await snapshot(visitor, 'merchant-brand', false)
        }
      }

      await save('home', { mode: 'html', html, url: null })
      await visitor.goto('/')
      const frame = visitor.frameLocator('iframe')
      await expect(frame.getByRole('heading', { name: '欢迎来到星河支付' })).toBeVisible()
      await expect(frame.locator('#parent-check')).toHaveText('SecurityError')
      await expect(frame.locator('#storage-check')).toHaveText('SecurityError')
      await expect(visitor.getByRole('banner')).toHaveCount(0)
      await expect(visitor.getByRole('contentinfo')).toHaveCount(0)
      for (const width of [360, 1280]) {
        await visitor.setViewportSize({ width, height: width === 360 ? 780 : 800 })
        for (const theme of ['light', 'dark'] as const) {
          await visitor.emulateMedia({ colorScheme: theme })
          await visitor.evaluate((value) => localStorage.setItem('vp-theme', value), theme)
          await visitor.reload()
          await expect(frame.getByRole('heading')).toBeVisible()
          await snapshot(visitor, 'home-html', false)
        }
      }

      // Give Chromium an HTTPS origin while still serving the real application and configuration.
      const selfOrigin = 'https://public-site-self.example.test'
      for (const suffix of ['/', '/?a=1']) {
        await save('home', { mode: 'url', html: null, url: `${selfOrigin}${suffix}` })
        const self = await visitorContext.newPage()
        const documents: string[] = []
        try {
          await self.route(`${selfOrigin}/**`, async (route) => {
            const request = route.request()
            if (request.resourceType() === 'document' && request.frame() === self.mainFrame()) {
              documents.push(request.url())
              if (documents.length > 1) return route.abort()
            }
            const { pathname, search } = new URL(request.url())
            const response = await route.fetch({ url: `${origin}${pathname}${search}` })
            await route.fulfill({ response })
          })
          await self.goto(`${selfOrigin}/`)
          await expect(self.getByRole('heading', { name: '用自己的收款码，收得清清楚楚' })).toBeVisible()
          await expect(self.getByRole('contentinfo').getByRole('link', { name: '帮助中心' })).toBeVisible()
          expect(documents).toEqual([`${selfOrigin}/`])
        } finally {
          await self.close()
        }
      }

      await save('home', { mode: 'url', html: null, url: destination })
      // Cancel the destination navigation so its fallback link stays on screen.
      const keepDocument = await visitorContext.newPage()
      try {
        await keepDocument.route(destination, (route) => route.abort('aborted'))
        const captures = process.env.WEB_VISUAL_POLISH === '1'
        for (const width of captures ? [360, 1280] : [1280]) {
          await keepDocument.setViewportSize({ width, height: width === 360 ? 780 : 800 })
          const themes = [['light', '浅色'], ['dark', '深色']] as const
          for (const [theme, label] of captures ? themes : themes.slice(0, 1)) {
            await keepDocument.goto('/console/login')
            await keepDocument.getByRole('button', { name: label, exact: true }).click()
            const cancelled = keepDocument.waitForEvent('requestfailed', (request) => request.url() === destination)
            await keepDocument.goto('/')
            await cancelled
            const link = keepDocument.getByRole('main').getByRole('link', { name: '如未自动跳转，请点击继续' })
            await expect(link).toHaveAttribute('href', destination)
            await expect(link).toHaveCSS('padding-left', '0px')
            await expect(link).toHaveCSS('padding-right', '0px')
            await expect(keepDocument.locator('html')).toHaveAttribute('data-theme', theme)
            if (captures) await polishShot(keepDocument, 'home-url-link', false, 'web-visual-polish-shots')
          }
        }
      } finally {
        await keepDocument.close()
      }
      await visitor.route(destination, (route) => route.fulfill({ contentType: 'text/html', body: '<h1>Redirect destination</h1>' }))
      await visitor.goto('/')
      await expect(visitor).toHaveURL(destination)
      await expect(visitor.getByRole('heading', { name: 'Redirect destination' })).toBeVisible()
    } finally {
      try {
        await restore()
      } finally {
        await visitorContext.close()
      }
    }
  })
}
