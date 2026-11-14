import type { components } from '@/api/schema'
import { replaceEqualDeep } from '@tanstack/react-query'
import { normalizeEmailDomainRule } from '@/lib/email-domain'

export type Schema = components['schemas']
export type Settings = Schema['Settings']
export type SettingsGroup = 'site' | 'home' | 'footer_legal' | 'mail' | 'security' | 'payment' | 'risk' | 'billing'
export type SettingsDrafts = Pick<Settings, 'site' | 'home' | 'footer_legal' | 'payment'> & {
  mail: Omit<Settings['mail'], 'password_configured' | 'masked_password'>
  security: Omit<Settings['security'], 'secret_configured'>
  billing: Pick<Settings['billing'], 'merchant_pid'>
  risk: {
    ip_daily_order_limit: string
    ip_blacklist: string
    name_block_words: string
    checkout_turnstile_enabled: boolean
  }
}

export const settingsGroups: SettingsGroup[] = [
  'site',
  'home',
  'footer_legal',
  'mail',
  'security',
  'payment',
  'risk',
  'billing',
]

export function settingsSearch(search: Record<string, unknown>): { group: SettingsGroup } {
  return { group: settingsGroups.find((group) => group === search.group) ?? 'site' }
}

export function writableGroup<K extends SettingsGroup>(settings: Settings, group: K): SettingsDrafts[K]
export function writableGroup(settings: Settings, group: SettingsGroup): SettingsDrafts[SettingsGroup] {
  switch (group) {
    case 'footer_legal':
      return {
        ...settings.footer_legal,
        items: (settings.footer_legal.items ?? []).toSorted((a, b) => a.sort - b.sort),
      }
    case 'mail': {
      const { host, port, username, from_address, from_name, tls_mode } = settings.mail
      return { host, port, username, from_address, from_name, tls_mode }
    }
    case 'security': {
      const { turnstile_site_key, turnstile_enabled, restrict_email_domains, email_domains, ban_contact, registration_review } =
        settings.security
      return {
        turnstile_site_key,
        turnstile_enabled,
        restrict_email_domains,
        email_domains: email_domains ?? [],
        ban_contact,
        registration_review,
      }
    }
    case 'billing':
      return { merchant_pid: settings.billing.merchant_pid }
    case 'risk':
      return {
        ip_daily_order_limit: String(settings.risk.ip_daily_order_limit),
        ip_blacklist: settings.risk.ip_blacklist.join('\n'),
        name_block_words: settings.risk.name_block_words.join('\n'),
        checkout_turnstile_enabled: settings.risk.checkout_turnstile_enabled,
      }
    default:
      return settings[group]
  }
}

export function settingsEqual(left: unknown, right: unknown) {
  return replaceEqualDeep(left, right) === left
}

export function byteLength(value: string) {
  return new TextEncoder().encode(value).length
}

export function settingsRequestFits(input: Schema['SettingsUpdateGroup']) {
  return byteLength(JSON.stringify(input)) <= 301 * 1024
}

export function riskSettingsValue(value: SettingsDrafts['risk']): Settings['risk'] {
  return {
    ip_daily_order_limit: Number(value.ip_daily_order_limit),
    ip_blacklist: value.ip_blacklist.split(/\r?\n/).map((line) => line.trim()).filter(Boolean),
    name_block_words: value.name_block_words.split(/\r?\n/).filter((line) => line.trim()),
    checkout_turnstile_enabled: value.checkout_turnstile_enabled,
  }
}

function publicURL(value: string, protocols: string[]) {
  try {
    const url = new URL(value)
    return protocols.includes(url.protocol) && !!url.hostname && !url.username && !url.password && !url.hash
  } catch {
    return false
  }
}

function validFooterURL(value: string) {
  if (/%(?![\da-f]{2})/i.test(value)) return false
  if (value.includes('#') || Array.from(value).some((char) => char.charCodeAt(0) < 32 || char === '\x7f'))
    return false
  if (/^mailto:/i.test(value)) return /^mailto:(?!\/)[^?]+(?:\?.*)?$/i.test(value)
  const authority = /^https?:\/\/([^/?#]+)/i.exec(value)?.[1]
  return (
    !!authority &&
    !authority.includes('@') &&
    !authority.includes('\\') &&
    publicURL(value, ['http:', 'https:'])
  )
}

export function validateSettingsGroup(
  group: SettingsGroup,
  draft: SettingsDrafts[SettingsGroup],
): Record<string, string> {
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
      if (!value.url || value.url.length > 2048 || !publicURL(value.url, ['https:']) || value.html !== null)
        errors.url = 'home_url'
    } else errors.mode = 'home_mode'
  }
  if (group === 'footer_legal') {
    const value = draft as Settings['footer_legal']
    if ((value.items?.length ?? 0) > 50) errors.items = 'footer_items'
    const ids = new Set<string>()
    value.items?.forEach((item, index) => {
      if (
        !item.id ||
        item.id.length > 64 ||
        ids.has(item.id) ||
        item.sort < 0 ||
        !item.label.zh_CN.trim() ||
        Array.from(item.label.zh_CN).length > 80 ||
        Array.from(item.label.en).length > 80 ||
        Array.from(item.text.zh_CN).length > 1000 ||
        Array.from(item.text.en).length > 1000
      )
        errors[`item_${index}`] = 'footer_item'
      if (item.url !== null && !validFooterURL(item.url)) errors[`item_url_${index}`] = 'footer_url'
      ids.add(item.id)
    })
    for (const kind of ['terms', 'privacy'] as const) {
      const item = value[kind]
      if (!item.zh_CN.trim() || byteLength(item.zh_CN) > 128 * 1024 || byteLength(item.en) > 128 * 1024)
        errors[kind] = 'legal_text'
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
    if (
      value.turnstile_site_key.length > 512 ||
      (value.turnstile_enabled && !value.turnstile_site_key.trim())
    )
      errors.turnstile_site_key = 'security_turnstile'
    const domains = value.email_domains ?? []
    if (domains.length > 100 || (value.restrict_email_domains && !domains.length))
      errors.email_domains = 'security_domains'
    if (domains.some((entry) => !validDomain(entry.trim().toLowerCase())))
      errors.email_domains = 'security_domains'
  }
  if (group === 'payment') {
    const value = draft as Settings['payment']
    if (!Number.isInteger(value.min_amount_cents) || value.min_amount_cents < 1)
      errors.min_amount_cents = 'payment_amount'
    if (
      !Number.isInteger(value.max_amount_cents) ||
      value.max_amount_cents < value.min_amount_cents ||
      value.max_amount_cents > 999999999
    )
      errors.max_amount_cents = 'payment_amount'
    if (!Number.isInteger(value.default_ttl_seconds) || value.default_ttl_seconds < 60)
      errors.default_ttl_seconds = 'payment_ttl'
    if (
      !Number.isInteger(value.max_ttl_seconds) ||
      value.max_ttl_seconds < value.default_ttl_seconds ||
      value.max_ttl_seconds > 86400
    )
      errors.max_ttl_seconds = 'payment_ttl'
    if (!Number.isInteger(value.float_min_cents) || value.float_min_cents < 0)
      errors.float_min_cents = 'payment_float'
    if (
      !Number.isInteger(value.float_max_cents) ||
      value.float_max_cents < value.float_min_cents ||
      value.float_max_cents > 999
    )
      errors.float_max_cents = 'payment_float'
    if (
      !Number.isInteger(value.float_default_cents) ||
      value.float_default_cents < value.float_min_cents ||
      value.float_default_cents > value.float_max_cents
    )
      errors.float_default_cents = 'payment_float'
    if (value.float_direction !== 'up' && value.float_direction !== 'down')
      errors.float_direction = 'payment_direction'
  }
  if (group === 'risk') {
    const input = draft as SettingsDrafts['risk']
    const value = riskSettingsValue(input)
    if (!/^\d+$/.test(input.ip_daily_order_limit) || value.ip_daily_order_limit > 2147483647)
      errors.ip_daily_order_limit = 'risk_ip_limit'
    if (value.ip_blacklist.length > 500) errors.ip_blacklist = 'risk_ip_blacklist'
    if (value.name_block_words.length > 200 || value.name_block_words.some((word) => Array.from(word).length > 50))
      errors.name_block_words = 'risk_name_words'
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
  return (
    !extra &&
    !!local &&
    local.length <= 64 &&
    !!domain &&
    domain.length <= 253 &&
    domain.split('.').every((part) => !!part && part.length <= 63 && /^[a-z0-9-]+$/i.test(part))
  )
}

function validDomain(value: string) {
  return normalizeEmailDomainRule(value) !== null
}

export function invalidDomainLine(text: string) {
  return text.split(/\r?\n/).findIndex((line) => {
    const rule = line.trim()
    return rule !== '' && !validDomain(rule)
  }) + 1
}

export function normalizeDomains(text: string) {
  return text
    .split(/\r?\n/)
    .map((domain) => domain.trim())
    .filter(Boolean)
    .map((domain) => normalizeEmailDomainRule(domain) ?? domain)
}

export function newFooterItem(): Schema['SettingsFooterItem'] {
  return {
    id: crypto.randomUUID(),
    label: { zh_CN: '', en: '' },
    text: { zh_CN: '', en: '' },
    url: null,
    enabled: true,
    sort: 0,
  }
}
