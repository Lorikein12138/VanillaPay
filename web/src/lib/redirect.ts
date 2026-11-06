// Browsers normalize backslashes and control characters in URLs. Reject them
// before parsing, including their encoded forms, rather than relying on origin alone.
export function safePath(value: unknown): string | undefined {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) return undefined
  let decoded: string
  try { decoded = decodeURIComponent(value) } catch { return undefined }
  if (/[\\\s]/.test(decoded) || Array.from(decoded).some((char) => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127) || decoded.startsWith('//')) return undefined
  const url = new URL(decoded, 'https://local.invalid')
  if (url.origin !== 'https://local.invalid' || url.pathname === '/console' || url.pathname.startsWith('/console/')) return undefined
  if (url.pathname === '/login') return undefined
  return value
}

export function loginDestination(redirect: unknown, nextPath: unknown): string {
  return safePath(redirect) ?? safePath(nextPath) ?? '/dashboard'
}

export function consoleSafePath(value: unknown): string | undefined {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) return undefined
  let decoded: string
  try { decoded = decodeURIComponent(value) } catch { return undefined }
  // Reject nested encodings as well: no second decoder may turn a path into a URL.
  if (/[\\\s%]/.test(decoded) || Array.from(decoded).some((char) => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127)) return undefined
  const url = new URL(decoded, 'https://local.invalid')
  if (url.origin !== 'https://local.invalid' || !(url.pathname === '/console' || url.pathname.startsWith('/console/'))) return undefined
  if (['/console/login', '/console/setup', '/console/recovery'].some((path) => url.pathname === path || url.pathname.startsWith(path + '/'))) return undefined
  return value
}

export function consoleDestination(redirect: unknown, nextPath: unknown): string {
  return consoleSafePath(redirect) ?? consoleSafePath(nextPath) ?? '/console'
}
