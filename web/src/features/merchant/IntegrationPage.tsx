import { integrationQuery } from '@/app/portal-queries'
import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { api, APIError } from '@/api/client'
import { useEmailCode } from '@/features/auth/account-shared'
import { Link } from '@tanstack/react-router'
import { Button, buttonVariants } from '@/components/ui/button'
import { FormHint } from '@/components/ui/auth-screen'
import { sortChannels } from '@/components/ui/payment'
import { Select, SelectOption } from '@/components/ui/select'
import { CopyButton, CopyField } from '@/components/ui/copy-button'
import { formatCountdown } from '@/components/ui/countdown'
import { Dialog } from '@/components/ui/dialog'
import { Field, FieldGrid, FormActions } from '@/components/ui/field'
import { Checkbox, Input, InputGroup, Textarea } from '@/components/ui/input'
import { Card, Mono, Page, PageHeader, SectionHeader, Skeleton, SubsectionTitle } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { invalidateCollection, validWhitelist, yuanToCents, type Schema } from './collection-data'
import { formatOrderTime } from './OrderParts'
import { ModuleError, useLocalAction } from './CollectionParts'

export function IntegrationPage({ user }: { user: Schema['AuthUserDTO'] }) {
  const { t } = useTranslation()
  const name = user.username, client = useQueryClient()
  const integration = useQuery(integrationQuery(name))
  const [dialog, setDialog] = useState<'reset' | 'rsa' | null>(null)
  const [reveal, setReveal] = useState(false), [currentKey, setCurrentKey] = useState('')
  const secretAction = useLocalAction(), revealIntent = useRef(false)
  useEffect(() => () => { revealIntent.current = false }, [])
  function hide() { revealIntent.current = false; setReveal(false); setCurrentKey('') }
  const data = integration.data
  return <Page>
    <PageHeader title={t('polish.integration')} description={t('polish.managePaymentEndpointsKeysAndAccessControl')} />
    {integration.isPending && <Skeleton className="h-80" />}{integration.isError && <ModuleError error={integration.error} retry={() => { void integration.refetch() }} />}
    {data && <>
      <Card><SectionHeader title={t('polish.merchantCredentials')} /><div className="grid gap-5 sm:grid-cols-2"><div><p className="mb-2 text-caption text-text-3">PID</p><CopyField value={data.pid} /></div><div><p className="mb-2 text-caption text-text-3">{t('polish.keyVersion')}</p><p className="flex h-11 items-center text-label text-text">{data.key_version}</p></div><div className="sm:col-span-2"><p className="mb-2 text-caption text-text-3">{t('polish.apiBaseUrl')}</p><CopyField value={data.base_url} /></div></div>
        <div className="mt-5 border-t-[0.5px] border-border pt-5"><p className="mb-2 text-caption text-text-3">{t('polish.merchantKey')}</p><Mono>{reveal ? currentKey : '••••••••••••••••••••••••••••••••'}</Mono><div className="mt-4 flex flex-wrap gap-2"><Button loading={secretAction.busy} onClick={() => {
          if (reveal) { hide(); return }
          revealIntent.current = true
          void secretAction.run(async () => { const result = await api.integration(); if (secretAction.active.current && revealIntent.current) { setCurrentKey(result.key); setReveal(true) } })
        }}>{secretAction.busy ? t('polish.loadingKey') : reveal ? t('polish.hideKey') : t('polish.showKey')}</Button>{reveal && <CopyButton value={currentKey} label={t('polish.copyKey')} />}<Button onClick={() => { hide(); setDialog('reset') }}>{t('polish.resetKey')}</Button></div></div>
        {secretAction.error != null && <ModuleError error={secretAction.error} />}
      </Card>
      <Card>
        <SectionHeader title={t('polish.apiGuide')} description={t('polish.v1SignsRequestsWithTheMerchantKey')} />
        <div className="grid gap-6 md:grid-cols-2">
          {(['v1', 'v2'] as const).map((version) => (
            <div key={version}>
              <SubsectionTitle className="mb-4">{version}</SubsectionTitle>
              <dl className="space-y-4">
                {Object.entries(data[version]).map(([operation, url]) => (
                  <div key={operation}>
                    <dt className="mb-1.5 text-caption text-text-3">{t(`merchant.integration.endpoints.${operation}`, { defaultValue: operation })}</dt>
                    <dd><CopyField value={url} /></dd>
                  </div>
                ))}
              </dl>
            </div>
          ))}
        </div>
      </Card>
      <Card>
        <SectionHeader title={t('polish.rsaPublicKeys')} />
        <div className="flex flex-col gap-4">
          <SubsectionTitle>{t('polish.merchantPublicKey')}</SubsectionTitle>
          {data.merchant_public_key
            ? <CopyField value={data.merchant_public_key} />
            : <p className="text-label text-text-3">{t('polish.notConfigured')}</p>}
          <div><Button onClick={() => setDialog('rsa')}>{t('polish.saveMerchantPublicKey')}</Button></div>
        </div>
        <div className="mt-6 border-t-[0.5px] border-border pt-6">
          <SubsectionTitle className="mb-4">{t('polish.platformPublicKeys')}</SubsectionTitle>
          {data.platform_public_keys.length ? data.platform_public_keys.map((item) => (
            <div key={item.id} className="mt-4 space-y-2">
              <p className="text-caption text-text-3">{t('polish.fingerprint')}<Mono>{item.id}</Mono></p>
              <CopyField value={item.pem} label={t('polish.copyPlatformPublicKey')} />
            </div>
          )) : <p className="text-label text-text-3">{t('polish.noPlatformPublicKeys')}</p>}
        </div>
      </Card>
      <WhitelistForm key={JSON.stringify(data.ip_whitelist)} name={name} value={data.ip_whitelist} />
      <NotifyDomainsCard name={name} />
      <CallbackSettingsCard name={name} />
      <TestPaymentCard name={name} />
    </>}
    {dialog && <SecurityDialog mode={dialog} user={user} onClose={() => setDialog(null)} onChanged={() => invalidateCollection(client, name)} />}
  </Page>
}

function NotifyDomainsCard({ name }: { name: string }) {
  const { t } = useTranslation()
  const query = useQuery({ queryKey: ['merchant', name, 'notify-domains'], queryFn: ({ signal }) => api.notifyDomains(signal) })
  return <Card aria-label={t('risk.domains.title')}>
    <SectionHeader title={t('risk.domains.title')} description={t('risk.domains.description')} />
    {query.isPending && <Skeleton className="h-32" />}
    {query.isError && <ModuleError error={query.error} retry={() => { void query.refetch() }} />}
    {query.data && <NotifyDomainsForm name={name} value={query.data} />}
  </Card>
}

function NotifyDomainsForm({ name, value }: { name: string; value: Schema['MerchantNotifyDomains'] }) {
  const { t } = useTranslation()
  const client = useQueryClient(), action = useLocalAction(), id = useId()
  const [enabled, setEnabled] = useState(value.enabled)
  const [text, setText] = useState(value.entries.join('\n'))
  const [invalid, setInvalid] = useState(false)
  const [saved, setSaved] = useState(false)
  async function save() {
    const entries = text.split(/\r?\n/).map((entry) => entry.trim()).filter(Boolean)
    setSaved(false)
    setInvalid(entries.length > 20 || (enabled && entries.length === 0))
    if (entries.length > 20 || (enabled && entries.length === 0)) return
    await action.run(async () => {
      try {
        const result = await api.saveNotifyDomains({ enabled, entries })
        setText(result.entries.join('\n'))
        setSaved(true)
        client.setQueryData(['merchant', name, 'notify-domains'], result)
      } catch (error) {
        if (error instanceof APIError && error.problem?.fields?.entries) setInvalid(true)
        else throw error
      }
    })
  }
  return <form className="flex flex-col gap-5" onSubmit={(event) => { event.preventDefault(); void save() }}>
    <Checkbox checked={enabled} disabled={action.busy} onChange={(event) => { setEnabled(event.target.checked); setSaved(false) }}>{t('risk.domains.enable')}</Checkbox>
    <Field label={t('risk.domains.entries')} htmlFor={id} hint={t('risk.domains.hint')} error={invalid && t('risk.domains.invalid')}>
      <Textarea id={id} aria-describedby={`${id}-hint`} rows={5} className="font-mono" value={text} disabled={action.busy} onChange={(event) => { setText(event.target.value); setSaved(false) }} />
    </Field>
    {saved && <Notice tone="success">{t('risk.domains.saved')}</Notice>}
    {action.error != null && <ModuleError error={action.error} />}
    <FormActions className="border-t-[0.5px] border-border pt-5"><Button type="submit" variant="primary" loading={action.busy}>{t('risk.domains.save')}</Button></FormActions>
  </form>
}

export function SecurityDialog({ mode, user, onClose, onChanged }: { mode: 'reset' | 'rsa'; user: Schema['AuthUserDTO']; onClose: () => void; onChanged: () => Promise<void> }) {
  const { t } = useTranslation()
  const action = useLocalAction(), mail = useEmailCode(), id = useId()
  const [emailCode, setEmailCode] = useState(''), [totpCode, setTOTPCode] = useState(''), [pem, setPEM] = useState('')
  const [newKey, setNewKey] = useState<string | null>(null), [invalid, setInvalid] = useState(''), [saved, setSaved] = useState(false)
  const { remaining, sent } = mail
  function close() { setNewKey(null); setEmailCode(''); setTOTPCode(''); onClose() }
  async function send() {
    if (!user.email) return
    await mail.send({ email: user.email, purpose: 'reset_key' })
  }
  async function submit() {
    if (!emailCode.trim() || (user.totp_enabled && !/^\d{6}$/.test(totpCode))) { setInvalid(t('polish.enterTheEmailCodeAndTheSix')); return }
    if (mode === 'rsa' && !/^-----BEGIN (RSA )?PUBLIC KEY-----\s+[A-Za-z0-9+/=\s]+\s+-----END (RSA )?PUBLIC KEY-----$/.test(pem.trim())) { setInvalid(t('polish.enterAnRsaPublicKeyInPem')); return }
    setInvalid('')
    await action.run(async () => {
      const proof: Schema['AuthSecurityProof'] = { email_code: emailCode.trim(), ...(user.totp_enabled ? { totp_code: totpCode } : {}) }
      if (mode === 'reset') {
        const result = await api.resetKey(proof)
        if (action.active.current) setNewKey(result.key)
      } else { await api.saveRSA({ public_key_pem: pem.trim(), security_proof: proof }); if (action.active.current) setSaved(true) }
      setEmailCode(''); setTOTPCode(''); await onChanged()
    })
  }
  return <Dialog title={newKey !== null ? t('polish.newKeyGenerated') : mode === 'reset' ? t('polish.resetMerchantKey') : t('polish.saveRsaPublicKey')} description={newKey !== null ? t('polish.copyThisKeyNowAndStoreIt') : mode === 'reset' ? t('polish.resettingImmediatelyRevokesTheOldKeyVerify') : t('polish.verifyWithAnEmailCodeIssuedFor')} busy={action.busy || mail.busy} onClose={close} footer={newKey === null && !saved && <Button type="submit" form={id} variant="primary" loading={action.busy} disabled={mail.busy || !emailCode.trim() || (user.totp_enabled && !/^\d{6}$/.test(totpCode))}>{action.busy ? t('polish.submitting') : mode === 'reset' ? t('polish.confirmKeyReset') : t('polish.confirmPublicKeySave')}</Button>}>
    {newKey !== null ? <CopyField testId="one-time-key" value={newKey} label={t('polish.copyNewKey')} /> : saved ? <Notice>{t('polish.rsaPublicKeySaved')}</Notice> : <form id={id} className="flex flex-col gap-5" onSubmit={(event) => { event.preventDefault(); void submit() }}>
      {mode === 'rsa' && <Field label={t('polish.rsaPublicKeyPem')} htmlFor={id+'-pem'}><Textarea id={id+'-pem'} className="font-mono text-caption" rows={7} maxLength={60000} value={pem} disabled={action.busy} onChange={(event) => setPEM(event.target.value)} /></Field>}
      <p className="break-all text-label text-text-2">{t('polish.verificationEmail')}{user.email ?? t('polish.emailUnavailableRefreshYourSession')}</p>
      <Field label={t('polish.emailVerificationCode')} htmlFor={id+'-email'}><InputGroup action={<Button disabled={!user.email || mail.busy || action.busy || remaining > 0} loading={mail.busy} onClick={() => { void send() }}>{mail.busy ? t('polish.sending') : remaining > 0 ? formatCountdown(remaining) : t('polish.getEmailCode')}</Button>}><Input id={id+'-email'} autoComplete="one-time-code" inputMode="numeric" required value={emailCode} disabled={action.busy} onChange={(event) => setEmailCode(event.target.value)} /></InputGroup></Field>
      {sent && <Notice tone="info">{t('polish.verificationCodeSentCheckYourEmail')}</Notice>}
      {user.totp_enabled && <Field label={t('polish.authenticatorCode')} htmlFor={id+'-totp'}><Input id={id+'-totp'} autoComplete="off" inputMode="numeric" maxLength={6} required value={totpCode} disabled={action.busy} onChange={(event) => setTOTPCode(event.target.value)} /></Field>}
      {invalid ? <Notice tone="error">{invalid}</Notice> : (mail.error != null || action.error != null) && <ModuleError error={mail.error ?? action.error} />}
    </form>}
  </Dialog>
}

function WhitelistForm({ name, value }: { name: string; value: Schema['MerchantIPWhitelist'] }) {
  const { t } = useTranslation()
  const client = useQueryClient(), action = useLocalAction(), id = useId()
  const [enabled, setEnabled] = useState(value.enabled), [text, setText] = useState((value.entries ?? []).join('\n')), [invalid, setInvalid] = useState(false)
  const [pending, setPending] = useState<Schema['MerchantIPWhitelist'] | null>(null)
  return <Card><SectionHeader title={t('polish.ipAllowlist')} /><form className="flex flex-col gap-5" onSubmit={(event) => { event.preventDefault(); const entries = text.split(/\r?\n/).map((item) => item.trim()).filter(Boolean); if (!validWhitelist(entries, enabled)) { setInvalid(true); return }; setInvalid(false); setPending({ enabled, entries }) }}>
    <Checkbox checked={enabled} onChange={(event) => setEnabled(event.target.checked)} description={t('polish.enterOneIpv4Ipv6OrCidrPer')}>{t('polish.enableIpAllowlist')}</Checkbox>
    <Field label={t('polish.allowedIpAddressesOrCidrs')} htmlFor={id}><Textarea id={id} className="font-mono" rows={5} value={text} onChange={(event) => setText(event.target.value)} /></Field>{invalid && <Notice tone="error">{t('polish.enterUpTo100ValidIpAddresses')}</Notice>}<FormActions className="border-t-[0.5px] border-border pt-5"><Button type="submit" variant="primary">{t('polish.saveAllowlist')}</Button></FormActions>
  </form>{pending && <Dialog title={t('polish.confirmIpAllowlistSave')} description={t('polish.confirmThatYourServerSOutboundIp')} busy={action.busy} onClose={() => setPending(null)} footer={<Button variant="primary" loading={action.busy} onClick={() => { void action.run(async () => { await api.saveWhitelist(pending); setPending(null); await invalidateCollection(client, name) }) }}>{action.busy ? t('polish.submitting') : t('polish.confirmAllowlistSave')}</Button>}>{action.error != null && <ModuleError error={action.error} />}</Dialog>}</Card>
}

function CallbackSettingsCard({ name }: { name: string }) {
  const { t } = useTranslation()
  const query = useQuery({ queryKey: ['merchant', name, 'callback-settings'], queryFn: ({ signal }) => api.callbackSettings(signal) })
  return <Card aria-label={t('tools.callback.title')}>
    <SectionHeader title={t('tools.callback.title')} description={t('tools.callback.description')} />
    {query.isPending && <Skeleton className="h-20" />}
    {query.isError && <ModuleError error={query.error} retry={() => { void query.refetch() }} />}
    {query.data && <CallbackSettingsForm name={name} value={query.data} />}
  </Card>
}

function CallbackSettingsForm({ name, value }: { name: string; value: Schema['MerchantCallbackSettings'] }) {
  const { t } = useTranslation()
  const client = useQueryClient(), action = useLocalAction()
  const [hideName, setHideName] = useState(value.hide_name)
  const [saved, setSaved] = useState(false)
  return <form className="flex flex-col gap-5" onSubmit={(event) => {
    event.preventDefault()
    setSaved(false)
    void action.run(async () => {
      const result = await api.saveCallbackSettings({ hide_name: hideName })
      client.setQueryData(['merchant', name, 'callback-settings'], result)
      setHideName(result.hide_name)
      setSaved(true)
    })
  }}>
    <Checkbox checked={hideName} disabled={action.busy} description={t('tools.callback.hideNameHint')} onChange={(event) => { setHideName(event.target.checked); setSaved(false) }}>{t('tools.callback.hideName')}</Checkbox>
    {saved && <Notice tone="success">{t('tools.callback.saved')}</Notice>}
    {action.error != null && <ModuleError error={action.error} />}
    <FormActions><Button type="submit" variant="primary" loading={action.busy}>{t('tools.callback.save')}</Button></FormActions>
  </form>
}

const testPaymentErrors = ['SUBSCRIPTION_INACTIVE', 'AMOUNT_OUT_OF_RANGE', 'AMOUNT_CAPACITY_EXHAUSTED', 'DEVICE_OFFLINE'] as const

function TestPaymentCard({ name }: { name: string }) {
  const { t, i18n } = useTranslation()
  const client = useQueryClient(), action = useLocalAction(), id = useId()
  const [amount, setAmount] = useState('0.01'), [channel, setChannel] = useState('')
  const [invalid, setInvalid] = useState(false)
  const [created, setCreated] = useState<Schema['MerchantTestPayment'] | null>(null)
  const problem = action.error instanceof APIError ? action.error.problem?.code : undefined
  const code = testPaymentErrors.find((item) => item === problem)
  async function create() {
    const cents = yuanToCents(amount.trim())
    const valid = cents !== null && cents >= 1 && cents <= 999999999
    setInvalid(!valid)
    if (!valid) return
    setCreated(null)
    await action.run(async () => {
      const result = await api.testPayment({ amount_cents: cents, channel: channel === 'alipay' || channel === 'wxpay' ? channel : null })
      if (action.active.current) setCreated(result)
      await Promise.all([
        client.invalidateQueries({ queryKey: ['merchant', name, 'orders'] }),
        client.invalidateQueries({ queryKey: ['merchant', name, 'stats'] }),
      ])
    })
  }
  return <Card aria-label={t('tools.testPayment.title')}>
    <SectionHeader title={t('tools.testPayment.title')} description={t('tools.testPayment.description')} />
    <form className="flex flex-col gap-5" onSubmit={(event) => { event.preventDefault(); void create() }}>
      <FieldGrid>
        <Field label={t('tools.testPayment.amount')} htmlFor={id + '-amount'} error={invalid && t('tools.testPayment.amountInvalid')}>
          <Input id={id + '-amount'} inputMode="decimal" autoComplete="off" value={amount} disabled={action.busy} aria-invalid={invalid || undefined} onChange={(event) => { setAmount(event.target.value); setInvalid(false) }} />
        </Field>
        <Field label={t('tools.testPayment.channel')} htmlFor={id + '-channel'}>
          <Select id={id + '-channel'} value={channel} disabled={action.busy} onValueChange={setChannel}>
            <SelectOption value="">{t('tools.testPayment.channelAny')}</SelectOption>
            {sortChannels(['alipay', 'wxpay']).map((item) => <SelectOption key={item} value={item}>{t(`merchant.overview.channels.${item}`)}</SelectOption>)}
          </Select>
        </Field>
      </FieldGrid>
      <FormHint>{t('tools.testPayment.hint')}</FormHint>
      {created && <Notice tone="success" action={<span className="flex flex-wrap gap-2">
        <a href={created.cashier_url} target="_blank" rel="noopener noreferrer" className={buttonVariants({ variant: 'primary', size: 'sm' })}>{t('tools.testPayment.openCashier')}</a>
        <Link to="/orders/$tradeNo" params={{ tradeNo: created.trade_no }} className={buttonVariants({ size: 'sm' })}>{t('tools.testPayment.viewOrder')}</Link>
      </span>}>{t('tools.testPayment.created', { time: formatOrderTime(created.expires_at, i18n.resolvedLanguage) })}</Notice>}
      {action.error != null && (code ? <Notice tone="error">{t(`tools.testPayment.errors.${code}`)}</Notice> : <ModuleError error={action.error} />)}
      <FormActions><Button type="submit" variant="primary" loading={action.busy}>{t('tools.testPayment.create')}</Button></FormActions>
    </form>
  </Card>
}
