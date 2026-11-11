import { queryOptions, type QueryClient } from '@tanstack/react-query'
import { api, APIError, consoleApi, type Role } from '@/api/client'
import { sessionCookie } from '@/api/session-cookie'
import type { components } from '@/api/schema'

type Session = components['schemas']['SessionIdentity']
const sessionFreshness = 5 * 60_000

export function sessionQuery(role: Role) {
  return queryOptions({
    queryKey: [role, 'session'] as const,
    staleTime: sessionFreshness,
    gcTime: 10 * 60_000,
    refetchInterval: sessionFreshness,
    refetchOnWindowFocus: true,
    queryFn: async ({ client, signal, queryKey }) => {
      const accountApi = role === 'console' ? consoleApi : api
      while (!signal.aborted) {
        const cookie = sessionCookie(role)
        try {
          const session = await accountApi.me(false, signal)
          if (signal.aborted) break
          if (cookie !== sessionCookie(role)) continue
          if (cookie && session.csrf_token !== cookie) throw new APIError(0)
          const previous = client.getQueryData<Session>(queryKey)
          if (previous && (previous.user.id !== session.user.id || previous.csrf_token !== session.csrf_token)) {
            client.removeQueries({ queryKey: [role], predicate: (query) => query.queryKey.length !== 2 || query.queryKey[1] !== 'session' })
          }
          return { ...session, cookie }
        } catch (error) {
          // Logout and login can both change the cookie while verification is in flight.
          if (signal.aborted || cookie === sessionCookie(role)) throw error
        }
      }
      throw new APIError(0)
    },
  })
}

export function clearPortalSession(client: QueryClient, role: Role) {
  // Removing a query cancels its in-flight result, even for APIs without an abort signal.
  client.removeQueries({ queryKey: [role] })
}

export function setPortalSession(client: QueryClient, role: Role, session: Session) {
  clearPortalSession(client, role)
  // Bind the login result to the cookie issued with it, even if another tab has since replaced it.
  client.setQueryData(sessionQuery(role).queryKey, { ...session, cookie: session.csrf_token })
}

export async function ensurePortalSession(client: QueryClient, role: Role) {
  const options = sessionQuery(role)
  const session = client.getQueryData(options.queryKey)
  const error = client.getQueryState(options.queryKey)?.error
  if (session && (session.cookie !== sessionCookie(role) || Date.parse(session.expires_at) <= Date.now())) {
    clearPortalSession(client, role)
  } else if (isSessionRejected(error)) {
    clearPortalSession(client, role)
    throw error
  }
  return client.ensureQueryData(options)
}

export function isSessionRejected(error: unknown): error is APIError {
  return error instanceof APIError && (error.status === 401 || (error.status === 403 && error.problem?.code === 'FORBIDDEN'))
}
