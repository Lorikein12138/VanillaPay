import { useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, useBlocker } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi } from '@/api/client'
import type { components } from '@/api/schema'
import { Button } from '@/components/ui/button'
import { Checkbox, Input, Select, Textarea } from '@/components/ui/input'
import { FileInput } from '@/components/ui/file-input'
import { Field, FieldGrid, FormActions } from '@/components/ui/field'
import { DetailList } from '@/components/ui/detail-list'
import { Dialog } from '@/components/ui/dialog'
import { Notice } from '@/components/ui/notice'
import { Card, Page, PageHeader, SectionHeader } from '@/components/ui/layout'
import { AccountError, AccountField } from '@/features/auth/account-shared'
import { centsToYuan, yuanToCents } from '@/features/merchant/collection-data'
import { normalizeDomains, newFooterItem, settingsDrafts, settingsDirty, settingsGroups, validateSettingsGroup, type Settings, type SettingsDrafts, type SettingsGroup } from './settings-data'

type S = components['schemas']
const labels: Record<SettingsGroup, string> = { site: 'site', home: 'home', footer_legal: 'footer', mail: 'mail', security: 'security', payment: 'payment', billing: 'billing' }
const keys: SettingsGroup[] = settingsGroups

function conflictItems(group: SettingsGroup, value: Settings[SettingsGroup], t: (key: string) => string) {
  const row = (label: string, value: unknown) => ({ label, value: value == null || value === '' ? '—' : JSON.stringify(value) ?? '—' })
  if (group === 'site') return [row(t('consoleSettings.siteName'), (value as Settings['site']).site_name), row(t('consoleSettings.logo'), (value as Settings['site']).logo_asset_id ? t('consoleSettings.configured') : t('consoleSettings.notConfigured'))]
  if (group === 'home') {
    const home = value as Settings['home']
    return [row(t('consoleSettings.homeMode'), t(`consoleSettings.homeModes.${home.mode}`)), row(t('consoleSettings.homeUrl'), home.url), row(t('consoleSettings.homeHtml'), home.html ? t('consoleSettings.contentConfigured') : '—')]
  }
  if (group === 'footer_legal') {
    const footer = value as Settings['footer_legal']
    return [row(t('consoleSettings.footerCount'), footer.items?.length ?? 0), row(t('consoleSettings.terms'), footer.terms.zh_CN), row(t('consoleSettings.privacy'), footer.privacy.zh_CN)]
  }
  if (group === 'mail') {
    const mail = value as Settings['mail']
    return [row(t('consoleSettings.mailHost'), mail.host), row(t('consoleSettings.mailPort'), mail.port), row(t('consoleSettings.mailAddress'), mail.from_address), row(t('consoleSettings.mailPassword'), mail.password_configured ? t('consoleSettings.secretConfiguredShort') : t('consoleSettings.secretNotConfigured'))]
  }
  if (group === 'security') {
    const security = value as Settings['security']
    return [row(t('consoleSettings.turnstileSite'), security.turnstile_site_key), row(t('consoleSettings.allowedHost'), security.allowed_hostname), row(t('consoleSettings.restrictDomains'), security.restrict_email_domains ? t('consoleSettings.enabled') : t('consoleSettings.disabled')), row(t('consoleSettings.turnstileSecret'), security.secret_configured ? t('consoleSettings.secretConfiguredShort') : t('consoleSettings.secretNotConfigured'))]
  }
  if (group === 'payment') {
    const payment = value as Settings['payment']
    return [row(t('consoleSettings.min'), centsToYuan(payment.min_amount_cents)), row(t('consoleSettings.max'), centsToYuan(payment.max_amount_cents)), row(t('consoleSettings.defaultTtl'), payment.default_ttl_seconds), row(t('consoleSettings.maxTtl'), payment.max_ttl_seconds), row(t('consoleSettings.floatDefault'), centsToYuan(payment.float_default_cents)), row(t('consoleSettings.floatDirection'), payment.float_direction)]
  }
  const billing = value as Settings['billing']
  return [row(t('consoleSettings.billingPid'), billing.merchant_pid), row(t('consoleSettings.billingKey'), billing.key_configured ? t('consoleSettings.secretConfiguredShort') : t('consoleSettings.secretNotConfigured')), row(t('consoleSettings.billingValidity'), billing.credential_valid ? t('consoleSettings.enabled') : t('consoleSettings.disabled'))]
}

export function ConsoleSettingsPage({ group, onGroupChange }: { group: SettingsGroup; onGroupChange: (group: SettingsGroup) => void }) {
  const { t } = useTranslation(), client = useQueryClient()
  const query = useQuery({ queryKey: ['console', 'settings'], queryFn: ({ signal }) => consoleApi.settings(signal) })
  const [drafts, setDrafts] = useState<SettingsDrafts | null>(null), [version, setVersion] = useState<number | null>(null)
  const [conflict, setConflict] = useState(false), [reviewed, setReviewed] = useState(false), [notice, setNotice] = useState(''), [error, setError] = useState<unknown>(null), [busy, setBusy] = useState(false)
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({})
  const [secretValues, setSecretValues] = useState<Partial<Record<'password' | 'turnstile_secret' | 'merchant_key', string | null>>>({})
  useEffect(() => () => setSecretValues({}), [group])
  const draftValues = drafts ?? (query.data ? settingsDrafts(query.data) : null)
  const currentVersion = version ?? query.data?.version ?? null
  const dirty = (!!draftValues && !!query.data && keys.some((key) => settingsDirty(draftValues[key], query.data[key]))) || Object.values(secretValues).some((value) => value !== undefined)
  const blocker = useBlocker({ shouldBlockFn: ({ next }) => next.pathname !== '/console/settings' && dirty, enableBeforeUnload: dirty, withResolver: true })
  function selectGroup(next: SettingsGroup) { setSecretValues({}); onGroupChange(next) }
  function updateSecret(key: 'password' | 'turnstile_secret' | 'merchant_key', value: string | null) {
    setFieldErrors({})
    setSecretValues((old) => {
      const next = { ...old }
      if (value === '') delete next[key]
      else next[key] = value
      return next
    })
  }
  function change<K extends SettingsGroup>(key: K, value: SettingsDrafts[K]) {
    if (!query.data) return
    setDrafts((current) => ({ ...(current ?? settingsDrafts(query.data)), [key]: value }))
    setFieldErrors({})
    if (version === null && query.data) setVersion(query.data.version)
  }
  async function save() {
    if (!draftValues || !query.data || currentVersion == null) return
    const errors = validateSettingsGroup(group, draftValues[group])
    if (group === 'billing' && secretValues.merchant_key && !/^[a-f0-9]{64}$/.test(secretValues.merchant_key)) errors.merchant_key = 'billing_key'
    if (Object.keys(errors).length) { setFieldErrors(errors); setError(null); return }
    setBusy(true); setError(null); setNotice('')
    const current = draftValues[group] as Record<string, unknown>
    const value: Record<string, unknown> = group === 'mail'
      ? { host: current.host, port: current.port, username: current.username, from_address: current.from_address, from_name: current.from_name, tls_mode: current.tls_mode }
      : group === 'security'
        ? { turnstile_site_key: current.turnstile_site_key, allowed_hostname: current.allowed_hostname, restrict_email_domains: current.restrict_email_domains, email_domains: current.email_domains, ban_contact: current.ban_contact }
        : group === 'billing' ? { merchant_pid: current.merchant_pid } : { ...current }
    if (group === 'mail' && secretValues.password !== undefined && secretValues.password !== '') value.password = secretValues.password
    if (group === 'security' && secretValues.turnstile_secret !== undefined && secretValues.turnstile_secret !== '') value.turnstile_secret = secretValues.turnstile_secret
    if (group === 'billing' && secretValues.merchant_key !== undefined && secretValues.merchant_key !== '') value.merchant_key = secretValues.merchant_key
    if (group === 'home') { const home = draftValues.home; value.html = home.mode === 'html' ? home.html : null; value.url = home.mode === 'url' ? home.url : null }
    if (group === 'footer_legal') value.items = draftValues.footer_legal.items?.map((item, sort) => ({ ...item, sort })) ?? []
    if (group === 'security') value.email_domains = normalizeDomains((draftValues.security.email_domains ?? []).join('\n'))
    const input = { expected_version: currentVersion, value } as S['SettingsUpdateGroup']
    try {
      const result = await consoleApi.saveSettings(group, input)
      const latest = client.getQueryData<Settings>(['console', 'settings']) ?? query.data
      let savedValue = result.value as Record<string, unknown>
      if (group === 'mail') savedValue = {
        ...savedValue,
        password_configured: secretValues.password === null ? false : secretValues.password ? true : latest.mail.password_configured,
        masked_password: latest.mail.masked_password,
      }
      if (group === 'security') savedValue = {
        ...savedValue,
        secret_configured: secretValues.turnstile_secret === null ? false : secretValues.turnstile_secret ? true : latest.security.secret_configured,
      }
      if (group === 'billing') savedValue = {
        ...savedValue,
        key_configured: secretValues.merchant_key === null ? false : secretValues.merchant_key ? true : latest.billing.key_configured,
        merchant_id: latest.billing.merchant_id,
        credential_valid: true,
      }
      const next = { ...latest, version: result.version, [group]: savedValue }
      client.setQueryData(['console', 'settings'], next); setVersion(result.version)
      await client.invalidateQueries({ queryKey: ['console', 'stats'] })
      await client.invalidateQueries({ queryKey: ['console', 'settings'] })
      setDrafts((old) => old ? { ...old, [group]: structuredClone(savedValue) as never } : old)
      setSecretValues({})
      setConflict(false); setReviewed(false)
      setNotice(group === 'payment' && result.affected_merchants > 0 ? t('consoleSettings.paymentSavedAffected', { count: result.affected_merchants }) : t(`consoleSettings.saved.${group}`))
    } catch (cause) {
      if (cause instanceof APIError && cause.status === 409 && cause.problem?.code === 'VERSION_CONFLICT') {
        const fresh = await consoleApi.settings(); client.setQueryData(['console', 'settings'], fresh); setVersion(fresh.version); setConflict(true); setReviewed(false)
      } else if (cause instanceof APIError && cause.status === 409 && cause.problem?.code === 'BILLING_CREDENTIAL_STALE') {
        setFieldErrors({ merchant_pid: 'billing_stale', merchant_key: 'billing_stale' }); setError(null)
      } else setError(cause)
    } finally { setBusy(false) }
  }
  if (query.isPending || !draftValues) return <Page><PageHeader title={t('console.nav.settings')} /><Card>{t('auth.loading')}</Card></Page>
  if (query.error) return <Page><PageHeader title={t('console.nav.settings')} /><AccountError error={query.error} /></Page>
  const tr = t as (key: string, options?: Record<string, unknown>) => string
  const groupLabel = tr(`consoleSettings.groups.${labels[group]}`)
  return <Page>
    <PageHeader title={t('console.nav.settings')} description={t('consoleSettings.description')} />
    {notice && <Notice tone="success" onDismiss={() => setNotice('')}>{notice}</Notice>}
    <div className="grid min-w-0 gap-5 lg:grid-cols-[220px_minmax(0,1fr)]">
      <nav aria-label={t('consoleSettings.groupsLabel')} className="hidden min-w-0 flex-col gap-1 lg:flex">
        {keys.map((key) => <button key={key} type="button" onClick={() => selectGroup(key)} className={`flex min-h-11 items-center justify-between rounded-control px-3 text-left text-label ${group === key ? 'bg-hover font-medium text-text' : 'text-text-2 hover:bg-hover'}`}>
          <span>{tr(`consoleSettings.groups.${labels[key]}`)}</span>{settingsDirty(draftValues[key], query.data?.[key]) && <span className="text-caption text-dot-wait">{t('consoleSettings.unsaved')}</span>}
        </button>)}
      </nav>
      <div className="min-w-0">
        <div className="mb-4 lg:hidden"><Select aria-label={t('consoleSettings.groupsLabel')} value={group} onChange={(e) => selectGroup(e.target.value as SettingsGroup)}>{keys.map((key) => <option key={key} value={key}>{tr(`consoleSettings.groups.${labels[key]}`)}{settingsDirty(draftValues[key], query.data?.[key]) ? ` · ${t('consoleSettings.unsaved')}` : ''}</option>)}</Select></div>
        <Card>
          <SectionHeader title={groupLabel} />
          {conflict && <><Notice tone="warning">{t('consoleSettings.versionConflict')}</Notice><DetailList columns={2} items={conflictItems(group, query.data[group], t as (key: string) => string)} /><Checkbox checked={reviewed} onChange={(e) => setReviewed(e.target.checked)}>{t('consoleSettings.reviewLatest')}</Checkbox></>}
          {Boolean(error) && <AccountError error={error} />}
          <GroupForm key={group} group={group} data={draftValues} settings={query.data} onChange={change} errors={fieldErrors} secrets={secretValues} setSecret={updateSecret} />
          <FormActions className="mt-6"><Button variant="primary" loading={busy} disabled={busy || (conflict && !reviewed)} onClick={() => void save()}>{tr(`consoleSettings.save.${labels[group]}`)}</Button></FormActions>
        </Card>
      </div>
    </div>
    {blocker.status === 'blocked' && <Dialog title={t('consoleSettings.leaveTitle')} onClose={blocker.reset} footer={<><Button onClick={blocker.reset}>{t('consoleSettings.stay')}</Button><Button variant="dangerOutline" onClick={blocker.proceed}>{t('consoleSettings.leave')}</Button></>}><p className="m-0 text-label text-text-2">{t('consoleSettings.leaveDescription')}</p></Dialog>}
  </Page>
}

function GroupForm({ group, data, settings, onChange, errors, secrets, setSecret }: { group: SettingsGroup; data: SettingsDrafts; settings: Settings; onChange: <K extends SettingsGroup>(key: K, value: SettingsDrafts[K]) => void; errors: Record<string, string>; secrets: Partial<Record<'password' | 'turnstile_secret' | 'merchant_key', string | null>>; setSecret: (key: 'password' | 'turnstile_secret' | 'merchant_key', value: string | null) => void }) {
  const { t } = useTranslation(), translate = t as (key: string) => string
  const fieldError = (key: string) => errors[key] ? translate(`consoleSettings.errors.${errors[key]}`) : undefined
  if (group === 'site') return <SiteForm value={data.site} onChange={(value) => onChange('site', value)} error={fieldError('site_name')} />
  if (group === 'home') return <HomeForm value={data.home} onChange={(value) => onChange('home', value)} errors={{ mode: fieldError('mode'), html: fieldError('html'), url: fieldError('url') }} />
  if (group === 'footer_legal') return <FooterForm value={data.footer_legal} onChange={(value) => onChange('footer_legal', value)} errors={errors} errorText={fieldError} />
  if (group === 'mail') return <MailForm value={data.mail} configured={settings.mail.password_configured} dirty={settingsDirty(data.mail, settings.mail) || secrets.password !== undefined} secret={secrets.password} onSecret={(value) => setSecret('password', value)} onChange={(value) => onChange('mail', value)} errorText={fieldError} />
  if (group === 'security') return <SecurityForm value={data.security} configured={settings.security.secret_configured} secret={secrets.turnstile_secret} onSecret={(value) => setSecret('turnstile_secret', value)} onChange={(value) => onChange('security', value)} errorText={fieldError} />
  if (group === 'payment') return <PaymentForm value={data.payment} onChange={(value) => onChange('payment', value)} errorText={fieldError} />
  return <BillingForm value={data.billing} settings={settings.billing} secret={secrets.merchant_key} onSecret={(value) => setSecret('merchant_key', value)} onChange={(value) => onChange('billing', value)} errorText={fieldError} />
}

function SiteForm({ value, onChange, error }: { value: S['SiteGroup']; onChange: (value: S['SiteGroup']) => void; error?: string }) {
  const { t } = useTranslation(), [uploadError, setUploadError] = useState(''), [uploading, setUploading] = useState(false)
  async function upload(file?: File) {
    if (!file) return
    if (!['image/png', 'image/jpeg'].includes(file.type) || file.size > 5 * 1024 * 1024) { setUploadError(t('consoleSettings.logoTypeSize')); return }
    setUploading(true); setUploadError('')
    try {
      const image = new Image(), url = URL.createObjectURL(file)
      await new Promise<void>((resolve, reject) => { image.onload = () => resolve(); image.onerror = () => reject(new Error('image')); image.src = url })
      URL.revokeObjectURL(url)
      if (image.width > 4096 || image.height > 4096) throw new Error('dimensions')
      const result = await consoleApi.uploadLogo(file); onChange({ ...value, logo_asset_id: result.asset_id })
    } catch (e) {
      if (e instanceof Error && e.message === 'dimensions') setUploadError(t('consoleSettings.logoDimensions'))
      else if (e instanceof APIError && e.problem?.code === 'UPLOAD_TOO_LARGE') setUploadError(t('consoleSettings.logoTypeSize'))
      else if (e instanceof APIError && e.problem?.code === 'UNSUPPORTED_MEDIA_TYPE') setUploadError(t('consoleSettings.logoTypeSize'))
      else setUploadError(t('consoleSettings.logoUploadFailed'))
    }
    finally { setUploading(false) }
  }
  return <div className="space-y-5"><AccountField label={t('consoleSettings.siteName')} value={value.site_name} maxLength={80} error={error} onChange={(e) => onChange({ ...value, site_name: e.target.value })} />
    <Field label={t('consoleSettings.logo')} htmlFor="logo-file" hint={t('consoleSettings.logoPending')}>
      <FileInput id="logo-file" accept="image/png,image/jpeg" disabled={uploading} onChange={(e) => void upload(e.target.files?.[0])} />
    </Field>
    {uploadError && <Notice tone="error">{uploadError}</Notice>}
    {value.logo_asset_id && <div className="flex items-center gap-4"><img className="size-16 rounded-control border border-border object-contain" src={`/media/${encodeURIComponent(value.logo_asset_id)}`} alt={t('consoleSettings.logo')} /><Button onClick={() => onChange({ ...value, logo_asset_id: null })}>{t('consoleSettings.removeLogo')}</Button></div>}
  </div>
}

function HomeForm({ value, onChange, errors }: { value: S['Home']; onChange: (value: S['Home']) => void; errors: Record<string, string | undefined> }) {
  const { t } = useTranslation()
  return <div className="space-y-5"><Field label={t('consoleSettings.homeMode')} htmlFor="home-mode" error={errors.mode}><Select id="home-mode" value={value.mode} onChange={(e) => onChange({ mode: e.target.value as S['Home']['mode'], html: null, url: null })}><option value="default">{t('consoleSettings.homeDefault')}</option><option value="html">HTML</option><option value="url">URL</option></Select></Field>
    {value.mode === 'html' && <Field label="HTML" htmlFor="home-html" hint={t('consoleSettings.homeHtmlHint')} error={errors.html}><Textarea id="home-html" rows={10} maxLength={262144} value={value.html ?? ''} onChange={(e) => onChange({ ...value, html: e.target.value, url: null })} /></Field>}
    {value.mode === 'url' && <AccountField label={t('consoleSettings.homeUrl')} type="url" value={value.url ?? ''} maxLength={2048} error={errors.url} onChange={(e) => onChange({ ...value, url: e.target.value, html: null })} />}
  </div>
}

function FooterForm({ value, onChange, errors, errorText }: { value: S['SettingsFooterLegal']; onChange: (value: S['SettingsFooterLegal']) => void; errors: Record<string, string>; errorText: (key: string) => string | undefined }) {
  const { t } = useTranslation(); const items = value.items ?? []
  function replaceItem(index: number, patch: Partial<S['SettingsFooterItem']>) { onChange({ ...value, items: items.map((item, i) => i === index ? { ...item, ...patch } : item) }) }
  return <div className="space-y-6"><div className="space-y-4">{items.map((item, index) => <div key={item.id} className="space-y-4 border-b-[0.5px] border-border pb-5">
    <FieldGrid><AccountField label={t('consoleSettings.footerZh')} value={item.label.zh_CN} error={errorText(`item_${index}`)} onChange={(e) => replaceItem(index, { label: { ...item.label, zh_CN: e.target.value } })} /><AccountField label={t('consoleSettings.footerEn')} value={item.label.en} error={errorText(`item_${index}`)} onChange={(e) => replaceItem(index, { label: { ...item.label, en: e.target.value } })}/><AccountField label={t('consoleSettings.footerText')} value={item.text} error={errorText(`item_${index}`)} onChange={(e) => replaceItem(index, { text: e.target.value })} /><AccountField label={t('consoleSettings.footerUrl')} value={item.url ?? ''} error={errorText(`item_${index}`)} onChange={(e) => replaceItem(index, { url: e.target.value || null })} /></FieldGrid>
    <div className="flex flex-wrap items-center gap-2"><Checkbox checked={item.enabled} onChange={(e) => replaceItem(index, { enabled: e.target.checked })}>{t('consoleSettings.enabled')}</Checkbox><Button size="sm" disabled={index === 0} onClick={() => { const next = [...items]; const previous = next[index - 1]!; next[index - 1] = next[index]!; next[index] = previous; onChange({ ...value, items: next }) }}>{t('consoleSettings.moveUp')}</Button><Button size="sm" disabled={index === items.length - 1} onClick={() => { const next = [...items]; const following = next[index + 1]!; next[index + 1] = next[index]!; next[index] = following; onChange({ ...value, items: next }) }}>{t('consoleSettings.moveDown')}</Button><Button size="sm" variant="dangerOutline" onClick={() => onChange({ ...value, items: items.filter((_, i) => i !== index) })}>{t('consoleSettings.remove')}</Button></div>
  </div>)}</div>{errors.items && <p className="m-0 text-caption text-danger">{errorText('items')}</p>}<Button disabled={items.length >= 50} onClick={() => onChange({ ...value, items: [...items, newFooterItem()] })}>{t('consoleSettings.addFooter')}</Button><Notice tone="info">{t('consoleSettings.legalVersionNotice')}</Notice>
    {(['terms', 'privacy'] as const).map((kind) => <div key={kind} className="space-y-3"><SectionHeader as="h3" title={t(`consoleSettings.${kind}`)} /><Field label={t('consoleSettings.zhContent')} htmlFor={`${kind}-zh`} error={errorText(kind)}><Textarea id={`${kind}-zh`} rows={6} value={value[kind].zh_CN} onChange={(e) => onChange({ ...value, [kind]: { ...value[kind], zh_CN: e.target.value } })} /></Field><Field label={t('consoleSettings.enContent')} htmlFor={`${kind}-en`}><Textarea id={`${kind}-en`} rows={6} value={value[kind].en} onChange={(e) => onChange({ ...value, [kind]: { ...value[kind], en: e.target.value } })} /></Field></div>)}
  </div>
}

function SecretField({ label, configured, value, onChange, allowClear = true, error }: { label: string; configured: boolean; value?: string | null; onChange: (value: string | null) => void; allowClear?: boolean; error?: string }) {
  const { t } = useTranslation(), [confirm, setConfirm] = useState(false)
  return <><AccountField label={label} type="password" autoComplete="new-password" value={value ?? ''} hint={configured ? t('consoleSettings.secretConfigured') : t('consoleSettings.secretNotConfigured')} error={error} onChange={(e) => onChange(e.target.value)} />
    {configured && allowClear && <Checkbox checked={value === null} onChange={(e) => { if (e.target.checked) setConfirm(true); else onChange('') }}>{t('consoleSettings.clearSecret')}</Checkbox>}
    {confirm && <Dialog title={t('consoleSettings.clearSecret')} onClose={() => setConfirm(false)} footer={<><Button onClick={() => setConfirm(false)}>{t('manage.cancel')}</Button><Button variant="dangerOutline" onClick={() => { onChange(null); setConfirm(false) }}>{t('consoleSettings.confirmClear')}</Button></>}><Notice tone="warning">{label === t('consoleSettings.turnstileSecret') ? t('consoleSettings.clearTurnstileWarning') : label === t('consoleSettings.billingKey') && value === null ? t('consoleSettings.billingClearNeedsPid') : t('consoleSettings.clearSecretWarning')}</Notice></Dialog>}</>
}

function MailForm({ value, configured, dirty, secret, onSecret, onChange, errorText }: { value: S['MailSettings']; configured: boolean; dirty: boolean; secret?: string | null; onSecret: (value: string | null) => void; onChange: (value: S['MailSettings']) => void; errorText: (key: string) => string | undefined }) {
  const { t } = useTranslation(), [recipient, setRecipient] = useState(''), [state, setState] = useState(''), [errorCode, setErrorCode] = useState(''), controller = useRef<AbortController | null>(null)
  useEffect(() => () => controller.current?.abort(), [])
  const patch = (p: Partial<S['MailSettings']>) => onChange({ ...value, ...p })
  async function send() {
    controller.current?.abort(); const current = new AbortController(); controller.current = current; setState('queued'); setErrorCode('')
    try { const queued = await consoleApi.testMail(recipient); const until = Date.now() + 60000
      while (!current.signal.aborted && Date.now() < until) { await new Promise((resolve) => setTimeout(resolve, 2000)); if (current.signal.aborted) return
        const status = await consoleApi.mailTestStatus(queued.delivery_id, current.signal); if (status.state === 'sent' || status.state === 'failed') { setState(status.state); setErrorCode(status.error_code ?? ''); return }
      }
      if (!current.signal.aborted) setState('timeout')
    } catch { if (!current.signal.aborted) setState('failed') }
  }
  return <div className="space-y-5"><FieldGrid><AccountField label={t('consoleSettings.mailHost')} value={value.host} error={errorText('host')} onChange={(e) => patch({ host: e.target.value })} /><AccountField label={t('consoleSettings.mailPort')} inputMode="numeric" value={value.port} error={errorText('port')} onChange={(e) => patch({ port: Number(e.target.value) })} /><AccountField label={t('consoleSettings.mailUsername')} value={value.username} error={errorText('username')} onChange={(e) => patch({ username: e.target.value })} /><AccountField label={t('consoleSettings.mailAddress')} type="email" value={value.from_address} error={errorText('from_address')} onChange={(e) => patch({ from_address: e.target.value })} /><AccountField label={t('consoleSettings.mailName')} value={value.from_name} error={errorText('from_name')} onChange={(e) => patch({ from_name: e.target.value })} /><Field label={t('consoleSettings.mailTls')} htmlFor="mail-tls" error={errorText('tls_mode')}><Select id="mail-tls" value={value.tls_mode} onChange={(e) => patch({ tls_mode: e.target.value as S['MailSettings']['tls_mode'] })}><option value="tls">TLS</option><option value="starttls">STARTTLS</option></Select></Field></FieldGrid>
    <SecretField label={t('consoleSettings.mailPassword')} configured={configured} value={secret} onChange={onSecret} />
    <SectionHeader as="h3" title={t('consoleSettings.testMail')} description={t('consoleSettings.testMailSavedConfig')} />
    <div className="flex min-w-0 flex-col gap-2 sm:flex-row"><div className="min-w-0 flex-1"><Input type="email" value={recipient} onChange={(e) => setRecipient(e.target.value)} placeholder={t('consoleSettings.recipient')} /></div><Button variant="primary" disabled={dirty || !recipient.includes('@')} onClick={() => void send()}>{t('consoleSettings.sendTest')}</Button></div>
    {dirty && <p className="m-0 text-caption text-text-3">{t('consoleSettings.saveBeforeTest')}</p>}{state && <Notice tone={state === 'sent' ? 'success' : state === 'failed' ? 'error' : 'info'}>{state === 'sent' ? t('consoleSettings.mailSent') : state === 'failed' ? errorCode === 'smtp_delivery_failed' ? t('consoleSettings.smtpFailed') : t('consoleSettings.mailFailed') : state === 'timeout' ? t('consoleSettings.mailTimeout') : t('consoleSettings.mailQueued')}</Notice>}
  </div>
}

function SecurityForm({ value, configured, secret, onSecret, onChange, errorText }: { value: S['SecuritySettings']; configured: boolean; secret?: string | null; onSecret: (value: string | null) => void; onChange: (value: S['SecuritySettings']) => void; errorText: (key: string) => string | undefined }) {
  const { t } = useTranslation(), [domains, setDomains] = useState((value.email_domains ?? []).join('\n'))
  return <div className="space-y-5"><FieldGrid><AccountField label={t('consoleSettings.turnstileSite')} value={value.turnstile_site_key} error={errorText('turnstile_site_key')} onChange={(e) => onChange({ ...value, turnstile_site_key: e.target.value })} /><AccountField label={t('consoleSettings.allowedHost')} value={value.allowed_hostname} error={errorText('turnstile_site_key')} onChange={(e) => onChange({ ...value, allowed_hostname: e.target.value })} /></FieldGrid><SecretField label={t('consoleSettings.turnstileSecret')} configured={configured} value={secret} onChange={onSecret} />
    <Checkbox checked={value.restrict_email_domains} onChange={(e) => onChange({ ...value, restrict_email_domains: e.target.checked })}>{t('consoleSettings.restrictDomains')}</Checkbox><Field label={t('consoleSettings.emailDomains')} htmlFor="domains" error={errorText('email_domains')}><Textarea id="domains" value={domains} onChange={(e) => { setDomains(e.target.value); onChange({ ...value, email_domains: normalizeDomains(e.target.value) }) }} /></Field>
    {(['zh_CN', 'en'] as const).map((lang) => <AccountField key={lang} label={lang === 'zh_CN' ? t('consoleSettings.banContactZh') : t('consoleSettings.banContactEn')} value={value.ban_contact[lang]} onChange={(e) => onChange({ ...value, ban_contact: { ...value.ban_contact, [lang]: e.target.value } })} />)}
  </div>
}

function PaymentForm({ value, onChange, errorText }: { value: S['SettingsGroupBounds']; onChange: (value: S['SettingsGroupBounds']) => void; errorText: (key: string) => string | undefined }) {
  const { t } = useTranslation(), [amounts, setAmounts] = useState({ min: centsToYuan(value.min_amount_cents), max: centsToYuan(value.max_amount_cents), floatMin: centsToYuan(value.float_min_cents), floatMax: centsToYuan(value.float_max_cents), floatDefault: centsToYuan(value.float_default_cents) })
  function amount(name: keyof typeof amounts, cents: keyof S['SettingsGroupBounds']) { return <AccountField label={t(`consoleSettings.${name}`)} inputMode="decimal" value={amounts[name]} error={errorText(cents)} onChange={(e) => { const next = { ...amounts, [name]: e.target.value }; setAmounts(next); const parsed = yuanToCents(e.target.value); onChange({ ...value, [cents]: parsed ?? Number.NaN }) }} /> }
  return <div className="space-y-5"><FieldGrid>{amount('min', 'min_amount_cents')}{amount('max', 'max_amount_cents')}{amount('floatMin', 'float_min_cents')}{amount('floatMax', 'float_max_cents')}{amount('floatDefault', 'float_default_cents')}<AccountField label={t('consoleSettings.defaultTtl')} inputMode="numeric" value={value.default_ttl_seconds} error={errorText('default_ttl_seconds')} onChange={(e) => onChange({ ...value, default_ttl_seconds: Number(e.target.value) })} /><AccountField label={t('consoleSettings.maxTtl')} inputMode="numeric" value={value.max_ttl_seconds} error={errorText('max_ttl_seconds')} onChange={(e) => onChange({ ...value, max_ttl_seconds: Number(e.target.value) })} /><Field label={t('consoleSettings.floatDirection')} htmlFor="float-direction" error={errorText('float_direction')}><Select id="float-direction" value={value.float_direction} onChange={(e) => onChange({ ...value, float_direction: e.target.value })}><option value="up">{t('consoleSettings.up')}</option><option value="down">{t('consoleSettings.down')}</option></Select></Field></FieldGrid></div>
}

function BillingForm({ value, settings, secret, onSecret, onChange, errorText }: { value: S['BillingSettings']; settings: S['BillingSettings']; secret?: string | null; onSecret: (value: string | null) => void; onChange: (value: S['BillingSettings']) => void; errorText: (key: string) => string | undefined }) {
  const { t } = useTranslation()
  return <div className="space-y-5">{!settings.credential_valid && <Notice tone="warning">{t('consoleSettings.billingInvalid')}</Notice>}{settings.merchant_id != null && <p className="m-0 text-label"><Link className="text-accent-text underline" to="/console/merchants/$id" params={{ id: String(settings.merchant_id) }}>{t('consoleSettings.billingMerchant', { id: settings.merchant_id })}</Link></p>}
    <AccountField label={t('consoleSettings.billingPid')} value={value.merchant_pid} maxLength={16} error={errorText('merchant_pid')} onChange={(e) => onChange({ ...value, merchant_pid: e.target.value })} />
    <SecretField label={t('consoleSettings.billingKey')} configured={settings.key_configured} value={secret} onChange={onSecret} allowClear={!value.merchant_pid} error={errorText('merchant_key')} />
    {settings.key_configured && value.merchant_pid && <p className="m-0 text-caption text-text-3">{t('consoleSettings.billingClearNeedsPid')}</p>}
  </div>
}
