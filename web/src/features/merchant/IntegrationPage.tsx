import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { api } from '@/api/client'
import { useEmailCode } from '@/features/auth/account-shared'
import { Button } from '@/components/ui/button'
import { CopyButton, CopyField } from '@/components/ui/copy-button'
import { formatCountdown } from '@/components/ui/countdown'
import { Dialog } from '@/components/ui/dialog'
import { Field, FormActions } from '@/components/ui/field'
import { Checkbox, Input, InputGroup, Textarea } from '@/components/ui/input'
import { Card, Mono, Page, PageHeader, SectionHeader, Skeleton, SubsectionTitle } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { invalidateCollection, validWhitelist, type Schema } from './collection-data'
import { ModuleError, useLocalAction } from './CollectionParts'

export function IntegrationPage({ user }: { user: Schema['AuthUserDTO'] }) {
  const { t } = useTranslation()
  const name = user.username, client = useQueryClient()
  const integration = useQuery({ queryKey: ['merchant', name, 'integration'], queryFn: async () => {
    const data = await api.integration()
    // This endpoint returns a secret. Discard it before entering the query cache.
    return { ...data, key: '' }
  }, gcTime: 0 })
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
                    <dt className="mb-1.5 text-caption text-text-3">{operation}</dt>
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
    </>}
    {dialog && <SecurityDialog mode={dialog} user={user} onClose={() => setDialog(null)} onChanged={() => invalidateCollection(client, name)} />}
  </Page>
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
