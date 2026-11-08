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
