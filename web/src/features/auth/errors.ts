import { APIError } from '@/api/client'

export function authErrorKey(error: unknown) {
  if (!(error instanceof APIError) || error.status === 0) return 'auth.errors.network' as const
  switch (error.problem?.code) {
    case 'INVALID_CREDENTIALS': return 'auth.errors.credentials' as const
    case 'SETUP_REQUIRED': return 'auth.errors.setup' as const
    case 'CHALLENGE_EXPIRED': return 'auth.errors.expired' as const
    case 'INVALID_2FA':
    case 'INVALID_CODE': return 'auth.errors.code' as const
    case 'TURNSTILE_FAILED': return 'auth.errors.turnstile' as const
    case 'CSRF_FAILED': return 'auth.errors.csrf' as const
    case 'ACCOUNT_BANNED': return 'auth.errors.banned' as const
  }
  if (error.status === 429) return 'auth.errors.rateLimit' as const
  if (error.status === 403) return 'auth.errors.forbidden' as const
  if (error.status === 401) return 'auth.errors.session' as const
  if (error.status === 400) return 'auth.errors.validation' as const
  return 'auth.errors.unavailable' as const
}
