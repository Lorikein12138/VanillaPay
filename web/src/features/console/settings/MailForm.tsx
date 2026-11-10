import { Select, SelectOption } from '@/components/ui/select'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldGrid } from '@/components/ui/field'
import { Notice } from '@/components/ui/notice'
import { Card, SectionHeader } from '@/components/ui/layout'
import { AccountField } from '@/features/auth/account-shared'
import type { SettingsDrafts } from '../settings-data'
import { SecretField, type SecretProps } from './SecretField'
import { useMailTest } from './useMailTest'

export function MailForm({
  value,
  configured,
  secret,
  onSecret,
  onChange,
  errorText,
}: SecretProps & {
  value: SettingsDrafts['mail']
  configured: boolean
  onChange: (value: SettingsDrafts['mail']) => void
  errorText: (key: string) => string | undefined
}) {
  const { t } = useTranslation()
  const patch = (fields: Partial<SettingsDrafts['mail']>) => onChange({ ...value, ...fields })
  return (
    <div className="space-y-5">
      <FieldGrid>
        <AccountField
          label={t('consoleSettings.mailHost')}
          value={value.host}
          error={errorText('host')}
          onChange={(e) => patch({ host: e.target.value })}
        />
        <AccountField
          label={t('consoleSettings.mailPort')}
          inputMode="numeric"
          value={value.port}
          error={errorText('port')}
          onChange={(e) => patch({ port: Number(e.target.value) })}
        />
        <AccountField
          label={t('consoleSettings.mailUsername')}
          value={value.username}
          error={errorText('username')}
          onChange={(e) => patch({ username: e.target.value })}
        />
        <AccountField
          label={t('consoleSettings.mailAddress')}
          type="email"
          value={value.from_address}
          error={errorText('from_address')}
          onChange={(e) => patch({ from_address: e.target.value })}
        />
        <AccountField
          label={t('consoleSettings.mailName')}
          value={value.from_name}
          error={errorText('from_name')}
          onChange={(e) => patch({ from_name: e.target.value })}
        />
        <Field label={t('consoleSettings.mailTls')} htmlFor="mail-tls" error={errorText('tls_mode')}>
          <Select
            id="mail-tls"
            value={value.tls_mode}
            onValueChange={(next) => patch({ tls_mode: next as SettingsDrafts['mail']['tls_mode'] })}
          >
            <SelectOption value="tls">TLS</SelectOption>
            <SelectOption value="starttls">STARTTLS</SelectOption>
          </Select>
        </Field>
      </FieldGrid>
      <SecretField
        label={t('consoleSettings.mailPassword')}
        configured={configured}
        secret={secret}
        onSecret={onSecret}
      />
    </div>
  )
}

export function MailTestCard({ dirty, busy }: { dirty: boolean; busy: boolean }) {
  const { t } = useTranslation()
  const [recipient, setRecipient] = useState('')
  const { state, send } = useMailTest()
  const messages = {
    queued: 'mailQueued',
    sent: 'mailSent',
    failed: 'mailFailed',
    timeout: 'mailTimeout',
  } as const
  return (
    <Card aria-label={t('consoleSettings.testMail')}>
      <SectionHeader title={t('consoleSettings.testMail')} description={t('consoleSettings.testMailSavedConfig')} />
      <fieldset disabled={busy} className="m-0 min-w-0 space-y-3 border-0 p-0">
        <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
          <Input
            className="min-w-0 sm:flex-1"
            type="email"
            value={recipient}
            onChange={(e) => setRecipient(e.target.value)}
            aria-label={t('consoleSettings.recipient')}
            placeholder={t('consoleSettings.recipient')}
          />
          <Button
            variant="primary"
            disabled={dirty || state === 'queued' || !recipient.includes('@')}
            onClick={() => void send(recipient)}
          >
            {t('consoleSettings.sendTest')}
          </Button>
        </div>
        {dirty && <p className="m-0 text-caption text-text-3">{t('consoleSettings.saveBeforeTest')}</p>}
        {state !== 'idle' && (
          <Notice tone={state === 'sent' ? 'success' : state === 'failed' ? 'error' : 'info'}>
            {t(`consoleSettings.${messages[state]}`)}
          </Notice>
        )}
      </fieldset>
    </Card>
  )
}
