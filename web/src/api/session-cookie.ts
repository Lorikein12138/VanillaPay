import type { Role } from './client'

const sessionChangeKey = 'vp-session-change'

export function sessionCookie(role: Role) {
  const name = `vp_csrf_${role}=`
  const cookies = document.cookie.split(/;\s*/)
  const match = cookies.find((cookie) => cookie.startsWith('__Host-' + name)) ??
    cookies.find((cookie) => cookie.startsWith(name))
  return match?.slice(match.indexOf('=') + 1) ?? ''
}

export function notifySessionCookieChange() {
  // Only send a change signal. Read the actual role cookie locally in each tab.
  try { localStorage.setItem(sessionChangeKey, crypto.randomUUID()) }
  catch { /* Cookie changes are also checked on navigation and focus. */ }
}

export function subscribeToSessionCookies(check: () => void) {
  const onStorage = (event: StorageEvent) => { if (event.key === sessionChangeKey) check() }
  window.addEventListener('storage', onStorage)
  window.addEventListener('focus', check)
  window.addEventListener('pageshow', check)
  document.addEventListener('visibilitychange', check)
  window.cookieStore?.addEventListener('change', check)
  return () => {
    window.removeEventListener('storage', onStorage)
    window.removeEventListener('focus', check)
    window.removeEventListener('pageshow', check)
    document.removeEventListener('visibilitychange', check)
    window.cookieStore?.removeEventListener('change', check)
  }
}
