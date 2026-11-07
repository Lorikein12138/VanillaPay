import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Trans, useTranslation } from 'react-i18next'
import { FormError } from '@/components/FormError'
import { Checkbox, Select, Textarea } from '@/components/ui/input'
import { Field, FieldGrid } from '@/components/ui/field'
import { Notice } from '@/components/ui/notice'
import { AccountField } from '@/features/auth/account-shared'
import { centsToYuan, yuanToCents } from '@/features/merchant/collection-data'
import { normalizeDomains, settingsEqual, type SettingsDrafts, type Schema as S } from '../settings-data'
import { SecretField, type SecretProps } from './SecretField'

export function SecurityForm({
  value,
  configured,
  secret,
  onSecret,
  onChange,
  errorText,
}: SecretProps & {
  value: SettingsDrafts['security']
  configured: boolean
  onChange: (value: SettingsDrafts['security']) => void
  errorText: (key: string) => string | undefined
}) {
  const { t } = useTranslation(),
    [domains, setDomains] = useState((value.email_domains ?? []).join('\n'))
  const domainError = errorText('email_domains')
  return (
    <div className="space-y-5">
      <FieldGrid>
        <AccountField
          label={t('consoleSettings.turnstileSite')}
          value={value.turnstile_site_key}
          error={errorText('turnstile_site_key')}
          onChange={(e) => onChange({ ...value, turnstile_site_key: e.target.value })}
        />
        <AccountField
          label={t('consoleSettings.allowedHost')}
          value={value.allowed_hostname}
          error={errorText('turnstile_site_key')}
          onChange={(e) => onChange({ ...value, allowed_hostname: e.target.value })}
        />
      </FieldGrid>
      <SecretField
        label={t('consoleSettings.turnstileSecret')}
        configured={configured}
        turnstile
        secret={secret}
        onSecret={onSecret}
      />
      <div className="space-y-2">
        <Checkbox
          checked={value.restrict_email_domains}
          onChange={(e) => onChange({ ...value, restrict_email_domains: e.target.checked })}
        >
          {t('consoleSettings.restrictDomains')}
        </Checkbox>
        {value.restrict_email_domains ? (
          <Field label={t('consoleSettings.emailDomains')} htmlFor="domains" error={domainError}>
            <Textarea
              id="domains"
              aria-describedby={domainError ? 'domains-hint' : undefined}
              value={
                settingsEqual(normalizeDomains(domains), value.email_domains)
                  ? domains
                  : (value.email_domains ?? []).join('\n')
              }
              onChange={(e) => {
                setDomains(e.target.value)
                onChange({ ...value, email_domains: normalizeDomains(e.target.value) })
              }}
            />
          </Field>
        ) : domainError && <FormError>{domainError}</FormError>}
      </div>
      {(['zh_CN', 'en'] as const).map((lang) => (
        <AccountField
          key={lang}
          label={lang === 'zh_CN' ? t('consoleSettings.banContactZh') : t('consoleSettings.banContactEn')}
          value={value.ban_contact[lang]}
          onChange={(e) =>
            onChange({ ...value, ban_contact: { ...value.ban_contact, [lang]: e.target.value } })
          }
        />
      ))}
    </div>
  )
}

export function PaymentForm({
  value,
  onChange,
  errorText,
}: {
  value: S['SettingsGroupBounds']
  onChange: (value: S['SettingsGroupBounds']) => void
  errorText: (key: string) => string | undefined
}) {
  const { t } = useTranslation(),
    [amounts, setAmounts] = useState({
      min: centsToYuan(value.min_amount_cents),
      max: centsToYuan(value.max_amount_cents),
      floatMin: centsToYuan(value.float_min_cents),
      floatMax: centsToYuan(value.float_max_cents),
      floatDefault: centsToYuan(value.float_default_cents),
    })
  function amount(name: keyof typeof amounts, cents: keyof S['SettingsGroupBounds']) {
    return (
      <AccountField
        label={t(`consoleSettings.${name}`)}
        inputMode="decimal"
        value={
          Object.is(yuanToCents(amounts[name]) ?? Number.NaN, value[cents])
            ? amounts[name]
            : centsToYuan(Number(value[cents]))
        }
        error={errorText(cents)}
        onChange={(e) => {
          const next = { ...amounts, [name]: e.target.value }
          setAmounts(next)
          const parsed = yuanToCents(e.target.value)
          onChange({ ...value, [cents]: parsed ?? Number.NaN })
        }}
      />
    )
  }
  return (
    <div className="space-y-5">
      <FieldGrid>
        {amount('min', 'min_amount_cents')}
        {amount('max', 'max_amount_cents')}
        {amount('floatMin', 'float_min_cents')}
        {amount('floatMax', 'float_max_cents')}
        {amount('floatDefault', 'float_default_cents')}
        <AccountField
          label={t('consoleSettings.defaultTtl')}
          inputMode="numeric"
          value={value.default_ttl_seconds}
          error={errorText('default_ttl_seconds')}
          onChange={(e) => onChange({ ...value, default_ttl_seconds: Number(e.target.value) })}
        />
        <AccountField
          label={t('consoleSettings.maxTtl')}
          inputMode="numeric"
          value={value.max_ttl_seconds}
          error={errorText('max_ttl_seconds')}
          onChange={(e) => onChange({ ...value, max_ttl_seconds: Number(e.target.value) })}
        />
        <Field
          label={t('consoleSettings.floatDirection')}
          htmlFor="float-direction"
          error={errorText('float_direction')}
        >
          <Select
            id="float-direction"
            value={value.float_direction}
            onChange={(e) => onChange({ ...value, float_direction: e.target.value })}
          >
            <option value="up">{t('consoleSettings.up')}</option>
            <option value="down">{t('consoleSettings.down')}</option>
          </Select>
        </Field>
      </FieldGrid>
    </div>
  )
}

export function BillingForm({
  value,
  settings,
  secret,
  onSecret,
  onChange,
  errorText,
}: SecretProps & {
  value: SettingsDrafts['billing']
  settings: S['BillingSettings']
  onChange: (value: SettingsDrafts['billing']) => void
  errorText: (key: string) => string | undefined
}) {
  const { t } = useTranslation()
  return (
    <div className="space-y-5">
      {!settings.credential_valid && <Notice tone="warning">{t('consoleSettings.billingInvalid')}</Notice>}
      <AccountField
        label={t('consoleSettings.billingPid')}
        value={value.merchant_pid}
        maxLength={16}
        hint={settings.merchant_id != null && (
          <Trans
            i18nKey="consoleSettings.billingMerchant"
            values={{ id: settings.merchant_id }}
            components={{
              merchant: (
                <Link
                  className="text-accent-text underline"
                  to="/console/merchants/$id"
                  params={{ id: String(settings.merchant_id) }}
                />
              ),
            }}
          />
        )}
        error={errorText('merchant_pid')}
        onChange={(e) => onChange({ ...value, merchant_pid: e.target.value })}
      />
      <SecretField
        label={t('consoleSettings.billingKey')}
        configured={settings.key_configured}
        secret={secret}
        onSecret={onSecret}
        allowClear={!value.merchant_pid}
        clearUnavailableHint={settings.key_configured && value.merchant_pid
          ? t('consoleSettings.billingClearNeedsPid')
          : undefined}
        error={errorText('merchant_key')}
      />
    </div>
  )
}
