import { toASCII, toUnicode } from 'tr46'
import type { components } from '@/api/schema'

const lookup = { checkBidi: true, checkHyphens: true, checkJoiners: true, useSTD3ASCIIRules: true }
const wildcardLookup = { ...lookup, checkBidi: false, checkHyphens: false, useSTD3ASCIIRules: false }
const dots = /[。．｡]/g
const ascii = /^[^\u0080-\u{10ffff}]*$/u
const validRuleDomain = (value: string) => !!value && Array.from(value).length <= 253 &&
  !value.startsWith('.') && !value.endsWith('.') && !value.includes('..')

function normalizeRuleLabel(label: string) {
  if (!label.includes('*')) return toASCII(label, lookup)
  if (/^xn--[a-z0-9*-]*$/i.test(label)) return label.toLowerCase()
  const result = toUnicode(label, wildcardLookup)
  if (result.error || /[^a-z0-9*\u0080-\u{10ffff}-]/u.test(result.domain) ||
    result.domain.split('*').length !== label.split('*').length) return null
  return result.domain
}

export function normalizeEmailDomainRule(value: string) {
  const rule = normalizeRule(value)
  if (rule && !ascii.test(rule) && /(^|\.)xn--/i.test(value.trim().replace(dots, '.'))) return null
  return rule
}

function normalizeRule(value: string) {
  const rule = value.trim().replace(dots, '.')
  if (!validRuleDomain(rule)) return null
  if (!rule.includes('*')) {
    const normalized = toASCII(rule, lookup)
    return normalized && validRuleDomain(normalized) ? normalized : null
  }
  const labels: string[] = []
  for (const label of rule.split('.')) {
    const normalized = normalizeRuleLabel(label)
    if (!normalized) return null
    labels.push(normalized)
  }
  if (labels.some((label) => !ascii.test(label))) {
    for (const [index, label] of labels.entries()) {
      if (label.includes('*')) continue
      const result = toUnicode(label, lookup)
      if (result.error) return null
      labels[index] = result.domain
    }
  }
  const normalized = labels.join('.')
  return validRuleDomain(normalized) && !/^\*+$/.test(normalized) ? normalized : null
}

export function allowedEmail(email: string, restriction: components['schemas']['PublicConfig']['email_domain_restriction']) {
  const [, input, extra] = email.trim().split('@')
  if (!input || extra !== undefined) return false
  const domain = toASCII(input.replace(dots, '.').replace(/\.$/, ''), lookup)
  if (!domain || domain.startsWith('.') || domain.endsWith('.') || domain.includes('..')) return false
  const domains = restriction.domains ?? []
  if (!restriction.enabled || !domains.length) return true
  return domains.some((item) => {
    const rule = normalizeRule(item.trim().replace(/^@/, ''))
    if (!rule) return false
    if (ascii.test(rule)) return matchesDomain(domain, rule)
    const unicode = toUnicode(domain, lookup)
    return !unicode.error && matchesDomain(unicode.domain, rule)
  })
}

function matchesDomain(domain: string, rule: string) {
  if (!rule.includes('*')) return domain === rule
  const parts = rule.split('*')
  const prefix = parts[0]!, suffix = parts.at(-1)!
  const end = domain.length - suffix.length
  if (end < prefix.length || !domain.startsWith(prefix) || !domain.endsWith(suffix)) return false
  let remaining = domain.slice(prefix.length, end)
  for (const part of parts.slice(1, -1)) {
    const index = remaining.indexOf(part)
    if (index < 0) return false
    remaining = remaining.slice(index + part.length)
  }
  return true
}
