import { describe, expect, it } from 'vitest'
import {
  byteLength,
  newFooterItem,
  normalizeDomains,
  settingsRequestFits,
  settingsSearch,
  validateSettingsGroup,
  writableGroup,
  type Settings,
} from '@/features/console/settings-data'
import { centsToYuan, yuanToCents } from '@/features/merchant/collection-data'

const settings: Settings = {
  version: 9,
  site: { site_name: 'VanillaPay', logo_asset_id: null, favicon_asset_id: null },
  home: { mode: 'default', html: null, url: null },
  footer_legal: { items: [], terms: { zh_CN: '条款', en: '' }, privacy: { zh_CN: '隐私', en: '' } },
  mail: {
    host: 'mail.example.test',
    port: 465,
    username: 'mailer',
    from_address: 'pay@example.test',
    from_name: 'Pay',
    tls_mode: 'tls',
    password_configured: true,
    masked_password: '••••••••',
  },
  security: {
    turnstile_site_key: '',
    turnstile_enabled: false,
    registration_review: false,
    restrict_email_domains: false,
    email_domains: [],
    ban_contact: { zh_CN: '', en: '' },
    secret_configured: true,
  },
  payment: {
    min_amount_cents: 1,
    max_amount_cents: 100000,
    default_ttl_seconds: 300,
    max_ttl_seconds: 3600,
    float_min_cents: 0,
    float_max_cents: 99,
    float_default_cents: 0,
    float_direction: 'up',
  },
  risk: { ip_daily_order_limit: 0, ip_blacklist: [], name_block_words: [], checkout_turnstile_enabled: false },
  billing: { merchant_pid: '', key_configured: false, merchant_id: null, credential_valid: true },
}

describe('console settings form data', () => {
  it('accepts a 301 KiB settings request and rejects one more byte', () => {
    const value = structuredClone(settings.footer_legal)
    value.terms = { zh_CN: '条'.repeat(40000), en: '款'.repeat(40000) }
    const request = { expected_version: settings.version, value }
    const limit = 301 * 1024
    value.privacy.en = 'a'.repeat(limit - byteLength(JSON.stringify(request)))
    expect(byteLength(JSON.stringify(request))).toBe(limit)
    expect(settingsRequestFits(request)).toBe(true)
    value.privacy.en += 'a'
    expect(byteLength(JSON.stringify(request))).toBe(limit + 1)
    expect(settingsRequestFits(request)).toBe(false)
  })

  it('falls back to site for an invalid group and excludes read-only values', () => {
    expect(settingsSearch({ group: 'root' })).toEqual({ group: 'site' })
    expect(writableGroup(settings, 'mail')).not.toHaveProperty('password_configured')
    expect(writableGroup(settings, 'mail')).not.toHaveProperty('masked_password')
    expect(writableGroup(settings, 'security')).not.toHaveProperty('secret_configured')
    expect(writableGroup(settings, 'billing')).toEqual({ merchant_pid: '' })
  })

  it('validates home mode payload combinations and URL restrictions', () => {
    const draft = { home: structuredClone(writableGroup(settings, 'home')) }
    draft.home = { mode: 'default', html: null, url: null }
    expect(validateSettingsGroup('home', draft.home)).toEqual({})
    draft.home = { mode: 'html', html: '<main>Hi</main>', url: null }
    expect(validateSettingsGroup('home', draft.home)).toEqual({})
    draft.home = { mode: 'url', html: null, url: 'https://user:pass@example.test/#x' }
    expect(validateSettingsGroup('home', draft.home)).toHaveProperty('url')
  })

  it('validates payment boundaries and converts yuan to cents', () => {
    const draft = { payment: structuredClone(writableGroup(settings, 'payment')) }
    expect(yuanToCents('12.34')).toBe(1234)
    expect(centsToYuan(1234)).toBe('12.34')
    draft.payment = {
      ...draft.payment,
      min_amount_cents: 1,
      max_amount_cents: 999999999,
      default_ttl_seconds: 60,
      max_ttl_seconds: 86400,
      float_min_cents: 0,
      float_max_cents: 999,
      float_default_cents: 999,
    }
    expect(validateSettingsGroup('payment', draft.payment)).toEqual({})
    draft.payment.max_ttl_seconds = 59
    expect(validateSettingsGroup('payment', draft.payment)).toHaveProperty('max_ttl_seconds')
  })

  it('generates sortable footer items and normalizes domains before submit', () => {
    const first = newFooterItem(),
      second = newFooterItem()
    expect(first.id).not.toBe(second.id)
    const draft = { footer_legal: structuredClone(writableGroup(settings, 'footer_legal')) }
    draft.footer_legal.items = [first, second]
    expect(validateSettingsGroup('footer_legal', draft.footer_legal)).toHaveProperty('item_0')
    draft.footer_legal.items = [
      { ...first, label: { zh_CN: '支持', en: 'Support' }, url: 'mailto:support@vanillapay.test' },
    ]
    expect(validateSettingsGroup('footer_legal', draft.footer_legal)).toEqual({})
    draft.footer_legal.items[0]!.url = 'mailto:support@vanillapay.test\r\nBcc:attacker@example.test'
    expect(validateSettingsGroup('footer_legal', draft.footer_legal)).toHaveProperty('item_url_0', 'footer_url')
    for (const url of [
      'https://example.test/path',
      'mailto:ops@example.test?subject=Help',
      'https://example.test/docs/%25',
      'https://example.test/docs/%E4%B8%AD',
    ]) {
      draft.footer_legal.items[0]!.url = url
      expect(validateSettingsGroup('footer_legal', draft.footer_legal)).toEqual({})
    }
    for (const url of [
      'mailto:ops@example.test#fragment',
      'mailto:ops@example.test#',
      'mailto:',
      'https:///path',
      'https://example.test/docs/%zz',
      'https://example.test/docs/%a',
      'mailto:ops@example.test?subject=%zz',
    ]) {
      draft.footer_legal.items[0]!.url = url
      expect(validateSettingsGroup('footer_legal', draft.footer_legal)).toEqual({ item_url_0: 'footer_url' })
    }
    expect(normalizeDomains(' Example.COM \n\nSHOP.example ')).toEqual(['example.com', 'shop.example'])
  })

  it('accepts valid group boundary values and rejects oversized names and invalid PID', () => {
    const draft = { site: { ...settings.site }, billing: writableGroup(settings, 'billing') }
    draft.site.site_name = 'x'.repeat(80)
    expect(validateSettingsGroup('site', draft.site)).toEqual({})
    draft.site.site_name = 'x'.repeat(81)
    expect(validateSettingsGroup('site', draft.site)).toHaveProperty('site_name')
    draft.billing.merchant_pid = '0123456789012345'
    expect(validateSettingsGroup('billing', draft.billing)).toHaveProperty('merchant_pid')
  })
})
