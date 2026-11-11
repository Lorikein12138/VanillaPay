import { settingsQuery } from '@/app/portal-queries'
import { useCallback, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { APIError, consoleApi } from '@/api/client'
import { refreshPublicConfig } from '@/lib/public-config'
import {
  normalizeDomains,
  settingsEqual,
  settingsGroups,
  settingsRequestFits,
  validateSettingsGroup,
  writableGroup,
  type Schema,
  type Settings,
  type SettingsDrafts,
  type SettingsGroup,
} from '../settings-data'

type SecretEdit = { kind: 'replace'; value: string } | { kind: 'clear' }
type Draft<K extends SettingsGroup> = {
  baseline: SettingsDrafts[K]
  baselineVersion: number
  value: SettingsDrafts[K]
  secret?: SecretEdit
}
type Drafts = { [K in SettingsGroup]?: Draft<K> }

const secretFields = { mail: 'password', security: 'turnstile_secret', billing: 'merchant_key' } as const
const { queryKey } = settingsQuery()

function hasConflict(entry: Draft<SettingsGroup>, value: SettingsDrafts[SettingsGroup], version: number) {
  return (
    !settingsEqual(entry.baseline, value) || (entry.secret !== undefined && version > entry.baselineVersion)
  )
}

// Keep the admin's edits while adopting fields changed only on the server.
// Arrays are edited as a unit (including footer order and the domain list).
function rebaseValue(base: unknown, draft: unknown, latest: unknown): unknown {
  if (settingsEqual(base, draft)) return latest
  if (!isObject(base) || !isObject(draft) || !isObject(latest)) return draft
  return Object.fromEntries(
    Object.keys(draft).map((key) => [key, rebaseValue(base[key], draft[key], latest[key])]),
  )
}

function isObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
}

export function useSettingsEditor(group: SettingsGroup) {
  const client = useQueryClient()
  const query = useQuery({
    ...settingsQuery(),
    refetchOnWindowFocus: 'always',
  })
  const settings = query.data
  const [drafts, setDrafts] = useState<Drafts>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [oversize, setOversize] = useState(false)
  const [refreshFailed, setRefreshFailed] = useState(false)
  const [saved, setSaved] = useState<{ group: SettingsGroup; affected: number } | null>(null)
  const [verification, setVerification] = useState<{ key: string; token: string }>()
  const [verificationAttempt, setVerificationAttempt] = useState(0)
  const lock = useRef(false)
  const minimumVersion = useRef(0)

  const draft = drafts[group]
  const latest = settings && writableGroup(settings, group)
  const conflicts = settings
    ? settingsGroups.filter((key) => {
        const entry = drafts[key]
        return entry && hasConflict(entry, writableGroup(settings, key), settings.version)
      })
    : []
  const conflict = conflicts.includes(group)
  const secret = draft?.secret
  const secretInput = secret?.kind === 'replace' ? secret.value : secret?.kind === 'clear' ? null : undefined

  const security = group === 'security' && settings ? groupValue('security', settings) : undefined
  const needsProof = !!security?.turnstile_enabled && !!settings && (
    !settings.security.turnstile_enabled || security.turnstile_site_key !== settings.security.turnstile_site_key || secret !== undefined
  )
  const hasSecret = secret?.kind === 'replace' ? !!secret.value.trim() : secret?.kind !== 'clear' && !!settings?.security.secret_configured
  const canVerify = needsProof && !!security?.turnstile_site_key.trim() && hasSecret
  const proofKey = `${settings?.version}:${verificationAttempt}`
  const proofToken = verification?.key === proofKey ? verification.token : ''
  const onProof = useCallback((token: string) => setVerification({ key: proofKey, token }), [proofKey])
  const resetProof = () => setVerificationAttempt((value) => value + 1)

  function clearErrors() {
    setError(null)
    setFieldErrors({})
    setOversize(false)
  }

  function edit<K extends SettingsGroup>(key: K, change: (draft: Draft<K>) => Draft<K>) {
    const current = client.getQueryData<Settings>(queryKey)
    if (!current || lock.current) return
    clearErrors()
    const serverValue = writableGroup(current, key)
    setDrafts((previous) => {
      const existing = previous[key]
      const next = change({
        ...existing,
        ...(existing && hasConflict(existing, serverValue, current.version)
          ? { baseline: existing.baseline, baselineVersion: existing.baselineVersion }
          : { baseline: serverValue, baselineVersion: current.version }),
        value: existing?.value ?? serverValue,
      })
      return { ...previous, [key]: next.secret || !settingsEqual(next.value, next.baseline) ? next : undefined }
    })
  }

  function groupValue<K extends SettingsGroup>(key: K, server: Settings): SettingsDrafts[K] {
    return drafts[key]?.value ?? writableGroup(server, key)
  }

  function update<K extends SettingsGroup>(key: K, patch: Partial<SettingsDrafts[K]>) {
    if (key === 'security' && settings) {
      const before = groupValue('security', settings)
      const next = { ...before, ...patch }
      if (before.turnstile_site_key !== next.turnstile_site_key || before.turnstile_enabled !== next.turnstile_enabled) resetProof()
    }
    edit(key, (entry) => ({ ...entry, value: { ...entry.value, ...patch } }))
  }

  function updateSecret(value: string | null) {
    if (!settings) return
    if (group === 'security') resetProof()
    edit(group, (entry) => ({
      ...entry,
      // A background GET may reach the cache before its configuration status is rendered.
      baselineVersion: Math.min(entry.baselineVersion, settings.version),
      secret: value === null ? { kind: 'clear' } : value ? { kind: 'replace', value } : undefined,
    }))
  }

  function acknowledge() {
    if (!draft || !latest || !settings || !conflict || lock.current) return
    // The home mode and its two mutually exclusive payload fields travel together.
    const value =
      group === 'home' && !settingsEqual(draft.value, draft.baseline)
        ? draft.value
        : (rebaseValue(draft.baseline, draft.value, latest) as SettingsDrafts[SettingsGroup])
    setDrafts((previous) => ({
      ...previous,
      [group]:
        draft.secret || !settingsEqual(value, latest)
          ? { baseline: latest, baselineVersion: settings.version, value, secret: draft.secret }
          : undefined,
    }))
    clearErrors()
  }

  async function refresh() {
    const result = await query.refetch()
    setRefreshFailed(!result.isSuccess || result.data.version < minimumVersion.current)
  }

  async function retryRefresh() {
    if (lock.current) return
    lock.current = true
    setBusy(true)
    try {
      await refresh()
    } finally {
      lock.current = false
      setBusy(false)
    }
  }

  async function save() {
    if (!settings || !latest || conflict || refreshFailed || lock.current) return
    const current = draft?.value ?? latest
    const errors = validateSettingsGroup(group, current)
    if (group === 'billing') {
      if (secret?.kind === 'replace' && !/^[a-f0-9]{64}$/.test(secret.value))
        errors.merchant_key = 'billing_key'
      if (secret?.kind === 'clear' && (current as SettingsDrafts['billing']).merchant_pid)
        errors.merchant_key = 'billing_clear'
    }
    if (security?.turnstile_enabled && !hasSecret) errors.turnstile_secret = 'security_secret'
    clearErrors()
    if (Object.keys(errors).length) {
      setFieldErrors(errors)
      return
    }
    if (needsProof && !proofToken) return
    const value = { ...current }
    if (group === 'footer_legal') {
      const items =
        (current as SettingsDrafts['footer_legal']).items?.map((item, sort) => ({ ...item, sort })) ?? []
      Object.assign(value, { items })
    }
    if (group === 'security') {
      const email_domains = normalizeDomains(
        ((current as SettingsDrafts['security']).email_domains ?? []).join('\n'),
      )
      Object.assign(value, { email_domains, ...(needsProof ? { turnstile_token: proofToken } : {}) })
    }
    if (secret && group in secretFields) {
      const field = secretFields[group as keyof typeof secretFields]
      Object.assign(value, { [field]: secret.kind === 'clear' ? null : secret.value })
    }
    const input: Schema['SettingsUpdateGroup'] = { expected_version: settings.version, value }
    if (!settingsRequestFits(input)) {
      setOversize(true)
      return
    }
    lock.current = true
    setBusy(true)
    setSaved(null)
    if (needsProof) resetProof()
    try {
      const result = await consoleApi.saveSettings(group, input)
      minimumVersion.current = result.version
      setDrafts((previous) => ({ ...previous, [group]: undefined }))
      if (group === 'site' || group === 'home' || group === 'footer_legal' || group === 'security') {
        void refreshPublicConfig(client).catch(() => {
          // The public query reports its own failure. The settings write has already succeeded.
        })
      }
      // Read all groups together before enabling writes at the new global version.
      // The PUT response does not provide authoritative credential configuration flags.
      await refresh()
      setSaved({ group, affected: result.affected_merchants })
    } catch (cause) {
      if (cause instanceof APIError && cause.status === 409 && cause.problem?.code === 'VERSION_CONFLICT') {
        await refresh()
      } else if (group === 'security' && cause instanceof APIError && cause.problem?.fields?.email_domains) {
        setFieldErrors({ email_domains: 'security_domains' })
      } else if (
        cause instanceof APIError &&
        cause.status === 409 &&
        cause.problem?.code === 'BILLING_CREDENTIAL_STALE'
      ) {
        setFieldErrors({ merchant_pid: 'billing_stale', merchant_key: 'billing_stale' })
      } else if (
        group === 'footer_legal' &&
        cause instanceof APIError &&
        cause.status === 400 &&
        cause.problem?.code === 'VALIDATION_ERROR'
      ) {
        setFieldErrors({ footer_legal: 'footer_url' })
      } else {
        setError(cause)
      }
    } finally {
      lock.current = false
      setBusy(false)
    }
  }

  return {
    settings,
    queryError: query.error,
    drafts,
    busy,
    conflict,
    error,
    fieldErrors,
    oversize,
    refreshFailed,
    saved,
    secretInput,
    canVerify,
    proofKey,
    proofToken,
    onProof,
    resetProof,
    groupValue,
    update,
    updateSecret,
    acknowledge,
    retryRefresh,
    save,
    clearErrors,
    dismissSaved: () => setSaved(null),
  }
}
