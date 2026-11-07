import type { components } from '@/api/schema'
import { yuanToCents } from '@/features/merchant/collection-data'

export type Schema = components['schemas']
export type Settings = Schema['Settings']
export type SettingsGroup = 'site' | 'home' | 'footer_legal' | 'mail' | 'security' | 'payment' | 'billing'
export type SettingsDrafts = Pick<Settings, SettingsGroup>

export const settingsGroups: SettingsGroup[] = ['site', 'home', 'footer_legal', 'mail', 'security', 'payment', 'billing']

export function settingsSearch(search: Record<string, unknown>): { group: SettingsGroup } {
  return { group: settingsGroups.find((group) => group === search.group) ?? 'site' }
}

export function settingsDrafts(settings: Settings): SettingsDrafts {
  return {
    site: structuredClone(settings.site),
    home: structuredClone(settings.home),
    footer_legal: structuredClone({ ...settings.footer_legal, items: settings.footer_legal.items ?? [] }),
    mail: structuredClone(settings.mail),
    security: structuredClone({ ...settings.security, email_domains: settings.security.email_domains ?? [] }),
    payment: structuredClone(settings.payment),
    billing: structuredClone(settings.billing),
  }
}

export function settingsDirty(draft: unknown, saved: unknown) {
  return JSON.stringify(draft) !== JSON.stringify(saved)
}

export function byteLength(value: string) { return new TextEncoder().encode(value).length }

function publicURL(value: string, protocols: string[]) {
  try {
    const url = new URL(value)
    return protocols.includes(url.protocol) && !!url.hostname && !url.username && !url.password && !url.hash
  } catch { return false }
}

export function validateSettingsGroup(group: SettingsGroup, draft: SettingsDrafts[SettingsGroup]): Record<string, string> {
  const errors: Record<string, string> = {}
  if (group === 'site') {
    const value = draft as Settings['site']
    if (!value.site_name.trim() || Array.from(value.site_name).length > 80) errors.site_name = 'site_name'
  }
  if (group === 'home') {
    const value = draft as Settings['home']
    if (value.mode === 'default') {
      if (value.html !== null || value.url !== null) errors.mode = 'home_mode'
    } else if (value.mode === 'html') {
      if (!value.html || byteLength(value.html) > 256 * 1024 || value.url !== null) errors.html = 'home_html'
    } else if (value.mode === 'url') {
      if (!value.url || value.url.length > 2048 || !publicURL(value.url, ['https:']) || value.html !== null) errors.url = 'home_url'
    } else errors.mode = 'home_mode'
  }
  if (group === 'footer_legal') {
    const value = draft as Settings['footer_legal']
    if ((value.items?.length ?? 0) > 50) errors.items = 'footer_items'
    const ids = new Set<string>()
    value.items?.forEach((item, index) => {
      if (!item.id || item.id.length > 64 || ids.has(item.id) || item.sort < 0 || !item.label.zh_CN.trim() || Array.from(item.label.zh_CN).length > 80 || Array.from(item.label.en).length > 80 || Array.from(item.text).length > 1000 || (item.url !== null && !publicURL(item.url, ['http:', 'https:']) && !/^mailto:[^\r\n]+$/i.test(item.url))) errors[`item_${index}`] = 'footer_item'
      ids.add(item.id)
    })
    for (const kind of ['terms', 'privacy'] as const) {
      const item = value[kind]
      if (!item.zh_CN.trim() || byteLength(item.zh_CN) > 128 * 1024 || byteLength(item.en) > 128 * 1024) errors[kind] = 'legal_text'
    }
  }
  if (group === 'mail') {
    const value = draft as Settings['mail']
    if (!value.host.trim() || value.host.length > 253) errors.host = 'mail_host'
    if (!Number.isInteger(value.port) || value.port < 1 || value.port > 65535) errors.port = 'mail_port'
    if (value.username.length > 254) errors.username = 'mail_username'
    if (!validEmail(value.from_address)) errors.from_address = 'mail_address'
    if (!value.from_name.trim() || Array.from(value.from_name).length > 120) errors.from_name = 'mail_name'
    if (value.tls_mode !== 'tls' && value.tls_mode !== 'starttls') errors.tls_mode = 'mail_tls'
  }
  if (group === 'security') {
    const value = draft as Settings['security']
    if (value.turnstile_site_key.length > 512 || value.allowed_hostname.length > 253 || ((value.turnstile_site_key === '') !== (value.allowed_hostname === ''))) errors.turnstile_site_key = 'security_turnstile'
    const domains = value.email_domains ?? []
    if (domains.length > 100 || (value.restrict_email_domains && !domains.length)) errors.email_domains = 'security_domains'
    if (domains.some((entry) => !validDomain(entry.trim().toLowerCase()))) errors.email_domains = 'security_domains'
  }
  if (group === 'payment') {
    const value = draft as Settings['payment']
    if (!Number.isInteger(value.min_amount_cents) || value.min_amount_cents < 1) errors.min_amount_cents = 'payment_amount'
    if (!Number.isInteger(value.max_amount_cents) || value.max_amount_cents < value.min_amount_cents || value.max_amount_cents > 999999999) errors.max_amount_cents = 'payment_amount'
    if (!Number.isInteger(value.default_ttl_seconds) || value.default_ttl_seconds < 60) errors.default_ttl_seconds = 'payment_ttl'
    if (!Number.isInteger(value.max_ttl_seconds) || value.max_ttl_seconds < value.default_ttl_seconds || value.max_ttl_seconds > 86400) errors.max_ttl_seconds = 'payment_ttl'
    if (!Number.isInteger(value.float_min_cents) || value.float_min_cents < 0) errors.float_min_cents = 'payment_float'
    if (!Number.isInteger(value.float_max_cents) || value.float_max_cents < value.float_min_cents || value.float_max_cents > 999) errors.float_max_cents = 'payment_float'
    if (!Number.isInteger(value.float_default_cents) || value.float_default_cents < value.float_min_cents || value.float_default_cents > value.float_max_cents) errors.float_default_cents = 'payment_float'
    if (value.float_direction !== 'up' && value.float_direction !== 'down') errors.float_direction = 'payment_direction'
  }
  if (group === 'billing') {
    const value = draft as Settings['billing']
    if (value.merchant_pid && !/^[1-9]\d{15}$/.test(value.merchant_pid)) errors.merchant_pid = 'billing_pid'
  }
  return errors
}

function validEmail(value: string) {
  if (value.length > 254 || /[\r\n\s<>(),;:\\[\]]/.test(value)) return false
  const [local, domain, extra] = value.split('@')
  return !extra && !!local && local.length <= 64 && !!domain && domain.length <= 253 && domain.split('.').every((part) => !!part && part.length <= 63 && /^[a-z0-9-]+$/i.test(part))
}

function validDomain(value: string) {
  return !!value && !/[ /:@*]/.test(value) && !value.startsWith('.') && !value.endsWith('.') && !value.includes('..')
}

export function normalizeDomains(text: string) {
  return text.split(/\r?\n/).map((domain) => domain.trim().toLowerCase()).filter(Boolean)
}

export function paymentDraftFromInputs(value: Settings['payment'], fields: { min: string; max: string; floatMin: string; floatMax: string; floatDefault: string }) {
  const min = yuanToCents(fields.min), max = yuanToCents(fields.max), floatMin = yuanToCents(fields.floatMin), floatMax = yuanToCents(fields.floatMax), floatDefault = yuanToCents(fields.floatDefault)
  if ([min, max, floatMin, floatMax, floatDefault].some((amount) => amount === null)) return null
  return { ...value, min_amount_cents: min!, max_amount_cents: max!, float_min_cents: floatMin!, float_max_cents: floatMax!, float_default_cents: floatDefault! }
}

export function newFooterItem(): Schema['SettingsFooterItem'] {
  return { id: crypto.randomUUID(), label: { zh_CN: '', en: '' }, text: '', url: null, enabled: true, sort: 0 }
}
