import { http, HttpResponse } from 'msw'
import type { components } from '@/api/schema'

export const defaultPublicConfig: components['schemas']['PublicConfig'] = {
  site_name: 'VanillaPay',
  logo_url: '',
  turnstile: { site_key: '', configured: false, login_action: 'login', register_action: 'register' },
  registration_open: true,
  email_domain_restriction: { enabled: false, domains: null },
  home: { mode: 'default', html: null, url: null },
  footer: [],
  legal_revisions: { terms: 0, privacy: 0 },
}

export const publicConfigHandler = http.get('*/api/v1/public/config', () => HttpResponse.json({ data: defaultPublicConfig }))

export function controlPublicConfigReads(snapshot: () => typeof defaultPublicConfig) {
  let releaseFirst!: () => void
  let releaseSecond!: () => void
  let firstReturning!: () => void
  const first = new Promise<void>((resolve) => { releaseFirst = resolve })
  const second = new Promise<void>((resolve) => { releaseSecond = resolve })
  const firstReturned = new Promise<void>((resolve) => { firstReturning = resolve })
  const reads: { signal: AbortSignal; data: typeof defaultPublicConfig }[] = []
  const handler = http.get('*/api/v1/public/config', async ({ request }) => {
    const data = snapshot()
    reads.push({ signal: request.signal, data })
    if (reads.length === 1) {
      await first
      firstReturning()
    } else if (reads.length === 2) await second
    return HttpResponse.json({ data })
  })
  return { handler, reads, releaseFirst, releaseSecond, firstReturned }
}
