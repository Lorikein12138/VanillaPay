// Foundation skeleton in a real browser: home, 404, error state, top bar, lazy HomePage.
// Runs against E2E_BASE_URL in every viewport project of tests/playwright.config.ts.
import { expect, test, type Page, type Request } from '@playwright/test'

const zhTitle = '用自己的收款码，收得清清楚楚'
const enTitle = 'Collect with your own payment codes, and know exactly what arrived'
const homeChunk = /\/assets\/HomePage-[0-9a-f]+\.js$/
const fontLicenses = ['source-serif-4', 'instrument-sans', 'noto-sans-sc', 'noto-serif-sc', 'jetbrains-mono']

/** Records every request the page makes so tests can assert document loads and lazy chunks. */
function trackRequests(page: Page) {
  const requests: Request[] = []
  page.on('request', (request) => requests.push(request))
  return {
    documents: () => requests.filter((request) => request.resourceType() === 'document' && request.frame() === page.mainFrame()),
    homeChunks: () => requests.filter((request) => homeChunk.test(new URL(request.url()).pathname)),
    external: (origin: string) => requests.filter((request) => !request.url().startsWith(origin) && !request.url().startsWith('data:')),
  }
}

/** Marks the current document; the mark survives client-side navigation but not a reload. */
async function markDocument(page: Page) {
  await page.evaluate(() => {
    document.documentElement.dataset.e2eDocument = 'kept'
  })
}

async function expectSameDocument(page: Page) {
  await expect(page.locator('html')).toHaveAttribute('data-e2e-document', 'kept')
}

async function expectNoHorizontalOverflow(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
  expect(overflow).toBeLessThanOrEqual(0)
}

function topBar(page: Page) {
  return page.getByRole('banner')
}

test.describe('foundation bootstrap', () => {
  test('home page lazy-loads in zh-CN with the shared top bar', async ({ page, baseURL }) => {
    const requests = trackRequests(page)
    await page.goto('/')

    await expect(page.getByRole('heading', { level: 1, name: zhTitle })).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
    await expect(topBar(page).getByRole('link', { name: 'VanillaPay 首页' })).toBeVisible()
    await expect(topBar(page).getByRole('button', { name: '切换语言为 English' })).toBeVisible()
    await expect(topBar(page).getByRole('group', { name: '主题' })).toBeVisible()
    expect(requests.homeChunks()).toHaveLength(1)
    expect(requests.documents()).toHaveLength(1)
    expect(requests.external(new URL(baseURL ?? '').origin)).toEqual([])
  })

  test('unknown path shows 404 without the home chunk and returns home in place', async ({ page }) => {
    const requests = trackRequests(page)
    await page.goto('/no-such-page')

    await expect(page.getByRole('heading', { level: 1, name: '页面不存在' })).toBeVisible()
    await expect(page.getByText('404', { exact: true })).toBeVisible()
    await expect(topBar(page).getByRole('button', { name: '切换语言为 English' })).toBeVisible()
    await expect(topBar(page).getByRole('group', { name: '主题' })).toBeVisible()
    expect(new URL(page.url()).pathname).toBe('/no-such-page')
    expect(requests.homeChunks()).toHaveLength(0)

    await markDocument(page)
    await page.getByRole('link', { name: '返回首页' }).click()

    await expect(page.getByRole('heading', { level: 1, name: zhTitle })).toBeVisible()
    expect(new URL(page.url()).pathname).toBe('/')
    expect(requests.homeChunks()).toHaveLength(1)
    expect(requests.documents()).toHaveLength(1)
    await expectSameDocument(page)
  })

  test('route error state replaces the page when the home chunk cannot load', async ({ page }) => {
    await page.route(homeChunk, (route) => route.abort())
    await page.goto('/')

    await expect(page.getByRole('heading', { level: 1, name: '页面出了点问题' })).toBeVisible()
    await expect(page.getByRole('button', { name: '重试' })).toBeVisible()
    await expect(topBar(page).getByRole('button', { name: '切换语言为 English' })).toBeVisible()
    await expect(page.getByRole('heading', { level: 1, name: zhTitle })).toHaveCount(0)
  })

  test('top bar switches language and theme without reloading and remembers both', async ({ page }) => {
    const requests = trackRequests(page)
    await page.goto('/')
    await expect(page.getByRole('heading', { level: 1, name: zhTitle })).toBeVisible()
    await markDocument(page)

    await topBar(page).getByRole('button', { name: '切换语言为 English' }).click()
    await expect(page.getByRole('heading', { level: 1, name: enTitle })).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('lang', 'en')

    const theme = topBar(page).getByRole('group', { name: 'Theme' })
    for (const [label, value] of [
      ['Dark', 'dark'],
      ['Light', 'light'],
    ] as const) {
      await theme.getByRole('button', { name: label }).click()
      await expect(page.locator('html')).toHaveAttribute('data-theme', value)
      await expect(theme.getByRole('button', { name: label })).toHaveAttribute('aria-pressed', 'true')
    }
    await theme.getByRole('button', { name: 'System' }).click()
    await expect(page.locator('html')).not.toHaveAttribute('data-theme')
    await theme.getByRole('button', { name: 'Dark' }).click()

    await expectSameDocument(page)
    expect(requests.documents()).toHaveLength(1)

    await page.reload()
    await expect(page.getByRole('heading', { level: 1, name: enTitle })).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await expect(topBar(page).getByRole('button', { name: 'Dark' })).toHaveAttribute('aria-pressed', 'true')
  })

  test('system theme follows the color scheme and matches the explicit themes', async ({ page }) => {
    const background = () => page.evaluate(() => getComputedStyle(document.body).backgroundColor)
    await page.goto('/')
    await expect(page.getByRole('heading', { level: 1, name: zhTitle })).toBeVisible()
    const theme = topBar(page).getByRole('group', { name: '主题' })

    await theme.getByRole('button', { name: '浅色' }).click()
    const light = await background()
    await theme.getByRole('button', { name: '深色' }).click()
    const dark = await background()
    expect(dark).not.toBe(light)

    await theme.getByRole('button', { name: '跟随系统' }).click()
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect.poll(background).toBe(dark)
    await page.emulateMedia({ colorScheme: 'light' })
    await expect.poll(background).toBe(light)
  })

  test('home and 404 fit the viewport in both languages without horizontal scrolling', async ({ page }) => {
    for (const language of ['zh-CN', 'en'] as const) {
      await page.addInitScript((value) => localStorage.setItem('vp-language', value), language)
      for (const path of ['/', '/no-such-page']) {
        await page.goto(path)
        await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
        await expect(page.locator('html')).toHaveAttribute('lang', language)
        await expectNoHorizontalOverflow(page)
      }
    }
  })

  test('english pages show no untranslated keys or chinese copy', async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem('vp-language', 'en'))
    for (const path of ['/', '/no-such-page']) {
      await page.goto(path)
      await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
      const text = await page.locator('body').innerText()
      expect(text).not.toMatch(/\b(?:common|home|language|theme|notFound|error)\.[A-Za-z]/)
      // The language switch names its target language in that language.
      expect(text.replaceAll('中文', '')).not.toMatch(/[㐀-鿿]/)
    }
  })

  test('keyboard reaches and operates every top bar control with a visible focus ring', async ({ page }) => {
    await page.goto('/no-such-page')
    await expect(page.getByRole('heading', { level: 1, name: '页面不存在' })).toBeVisible()

    const controls = [
      topBar(page).getByRole('link', { name: 'VanillaPay 首页' }),
      topBar(page).getByRole('button', { name: '切换语言为 English' }),
      topBar(page).getByRole('button', { name: '跟随系统' }),
      topBar(page).getByRole('button', { name: '浅色' }),
      topBar(page).getByRole('button', { name: '深色' }),
    ]
    for (const control of controls) {
      await page.keyboard.press('Tab')
      await expect(control).toBeFocused()
      const ring = await control.evaluate((element) => {
        const style = getComputedStyle(element)
        return { outline: style.outlineStyle, shadow: style.boxShadow }
      })
      expect(ring.outline).not.toBe('none')
      expect(ring.shadow).not.toBe('none')
    }

    await page.keyboard.press('Enter')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
    await page.keyboard.press('Shift+Tab')
    await page.keyboard.press('Space')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'light')

    await topBar(page).getByRole('button', { name: '切换语言为 English' }).focus()
    await page.keyboard.press('Enter')
    await expect(page.getByRole('heading', { level: 1, name: 'Page not found' })).toBeVisible()
    await expect(topBar(page).getByRole('button', { name: 'Switch language to 中文' })).toBeFocused()
  })

  test('reduced motion shortens transitions', async ({ page }) => {
    const duration = () =>
      topBar(page)
        .getByRole('button', { name: '切换语言为 English' })
        .evaluate((element) => Number.parseFloat(getComputedStyle(element).transitionDuration))
    await page.goto('/')
    await expect(page.getByRole('heading', { level: 1, name: zhTitle })).toBeVisible()
    expect(await duration()).toBeGreaterThan(0.05)

    await page.emulateMedia({ reducedMotion: 'reduce' })
    await expect.poll(duration).toBeLessThanOrEqual(0.001)
  })

  test('typography uses the self-hosted open-licensed fonts', async ({ page, request }) => {
    await page.goto('/')
    const heading = page.getByRole('heading', { level: 1, name: zhTitle })
    await expect(heading).toBeVisible()
    await page.evaluate(() => document.fonts.ready.then(() => undefined))

    expect(await heading.evaluate((element) => getComputedStyle(element).fontFamily)).toContain('Source Serif 4 Variable')
    expect(await page.locator('body').evaluate((element) => getComputedStyle(element).fontFamily)).toContain(
      'Instrument Sans Variable',
    )
    expect(await page.evaluate(() => document.fonts.check('16px "Noto Serif SC Variable"', '收款'))).toBe(true)

    for (const name of fontLicenses) {
      const response = await request.get(`/fonts/licenses/${name}.txt`)
      expect(response.ok()).toBe(true)
      expect(await response.text()).toContain('SIL OPEN FONT LICENSE Version 1.1')
    }
    expect(await page.locator('body').innerText()).not.toMatch(/anthropic|claude/i)
  })

  // Saves this run's home/404 screenshots for every language and theme at the project width, for manual review (V52).
  test('captures foundation review screenshots', async ({ page }, testInfo) => {
    const systemColorScheme = 'light'
    await page.emulateMedia({ colorScheme: systemColorScheme })
    testInfo.annotations.push({ type: 'system-color-scheme', description: systemColorScheme })
    const width = page.viewportSize()?.width
    await page.goto('/')

    for (const [name, path] of [
      ['home', '/'],
      ['404', '/no-such-page'],
    ] as const) {
      for (const language of ['zh-CN', 'en'] as const) {
        for (const theme of ['system', 'light', 'dark'] as const) {
          await page.evaluate(
            (preferences) => {
              localStorage.setItem('vp-language', preferences.language)
              localStorage.setItem('vp-theme', preferences.theme)
            },
            { language, theme },
          )
          await page.goto(path)
          await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
          await expect(page.locator('html')).toHaveAttribute('lang', language)
          if (theme === 'system') {
            await expect(page.locator('html')).not.toHaveAttribute('data-theme')
          } else {
            await expect(page.locator('html')).toHaveAttribute('data-theme', theme)
          }
          await page.evaluate(() => document.fonts.ready.then(() => undefined))

          const file = testInfo.outputPath(`foundation-${name}-${language}-${theme}-${width}.png`)
          await page.screenshot({ path: file, fullPage: true })
          testInfo.annotations.push({ type: 'screenshot', description: file })
        }
      }
    }
  })
})
