import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input, Select } from '@/components/ui/input'
import { Field, FieldGrid } from '@/components/ui/field'
import { Notice } from '@/components/ui/notice'
import { SectionHeader } from '@/components/ui/layout'
import { AccountField } from '@/features/auth/account-shared'
import type { SettingsDrafts } from '../settings-data'
import { SecretField, type SecretProps } from './SecretField'
import { useMailTest } from './useMailTest'

export function MailForm({
  value,
  configured,
  dirty,
  secret,
  onSecret,
  onChange,
  errorText,
}: SecretProps & {
  value: SettingsDrafts['mail']
  configured: boolean
  dirty: boolean
  onChange: (value: SettingsDrafts['mail']) => void
  errorText: (key: string) => string | undefined
}) {
  const { t } = useTranslation()
  const [recipient, setRecipient] = useState('')
  const { state, send } = useMailTest()
  const patch = (fields: Partial<SettingsDrafts['mail']>) => onChange({ ...value, ...fields })
  const messages = {
    queued: 'mailQueued',
    sent: 'mailSent',
    failed: 'mailFailed',
    timeout: 'mailTimeout',
  } as const
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
            onChange={(e) => patch({ tls_mode: e.target.value as SettingsDrafts['mail']['tls_mode'] })}
          >
            <option value="tls">TLS</option>
            <option value="starttls">STARTTLS</option>
          </Select>
        </Field>
      </FieldGrid>
      <SecretField
        label={t('consoleSettings.mailPassword')}
        configured={configured}
        secret={secret}
        onSecret={onSecret}
      />
      <SectionHeader
        as="h3"
        title={t('consoleSettings.testMail')}
        description={t('consoleSettings.testMailSavedConfig')}
      />
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row">
        <div className="min-w-0 flex-1">
          <Input
            type="email"
            value={recipient}
            onChange={(e) => setRecipient(e.target.value)}
            placeholder={t('consoleSettings.recipient')}
          />
        </div>
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
    </div>
  )
}
