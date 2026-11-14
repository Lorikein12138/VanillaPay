import { Select, SelectOption } from '@/components/ui/select'
import { useCallback, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Trans, useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Turnstile } from '@/features/auth/Turnstile'
import { FormError } from '@/components/FormError'
import { Checkbox, Input, Textarea } from '@/components/ui/input'
import { Field, FieldGrid } from '@/components/ui/field'
import { Notice } from '@/components/ui/notice'
import { AccountField } from '@/features/auth/account-shared'
import { centsToYuan, yuanToCents } from '@/features/merchant/collection-data'
import { invalidDomainLine, normalizeDomains, settingsEqual, type SettingsDrafts, type Schema as S } from '../settings-data'
import { SecretField, type SecretProps } from './SecretField'

export function SecurityForm({
  value,
  configured,
  secret,
  onSecret,
  onChange,
  errorText,
  proof,
}: SecretProps & {
  value: SettingsDrafts['security']
  configured: boolean
  proof?: { key: string; onToken: (token: string) => void; retry: () => void }
  onChange: (value: SettingsDrafts['security']) => void
  errorText: (key: string) => string | undefined
}) {
  const { t } = useTranslation(),
    [domains, setDomains] = useState((value.email_domains ?? []).join('\n'))
  const domainText = settingsEqual(normalizeDomains(domains), value.email_domains)
    ? domains
    : (value.email_domains ?? []).join('\n')
  const invalidLine = invalidDomainLine(domainText)
  const domainError = invalidLine ? t('consoleSettings.emailDomainLineError', { line: invalidLine }) : errorText('email_domains')
  return (
    <div className="space-y-5">
      <Checkbox checked={value.turnstile_enabled} onChange={(e) => onChange({ ...value, turnstile_enabled: e.target.checked })}>
        {t('consoleSettings.turnstileEnabled')}
      </Checkbox>
      <AccountField
        label={t('consoleSettings.turnstileSite')}
        value={value.turnstile_site_key}
        error={errorText('turnstile_site_key')}
        onChange={(e) => onChange({ ...value, turnstile_site_key: e.target.value })}
      />
      <SecretField
        label={t('consoleSettings.turnstileSecret')}
        configured={configured}
        turnstile
        secret={secret}
        onSecret={onSecret}
        error={errorText('turnstile_secret')}
      />
      {proof && <SecurityVerification key={proof.key} siteKey={value.turnstile_site_key} onToken={proof.onToken} retry={proof.retry} />}
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
              aria-invalid={!!domainError}
              aria-describedby={domainError ? 'domains-format domains-hint' : 'domains-format'}
              value={domainText}
              onChange={(e) => {
                setDomains(e.target.value)
                onChange({ ...value, email_domains: normalizeDomains(e.target.value) })
              }}
            />
            <p id="domains-format" className="m-0 text-caption leading-relaxed text-text-3">
              {t('consoleSettings.emailDomainFormat')}
            </p>
          </Field>
        ) : domainError && <FormError>{domainError}</FormError>}
      </div>
      <Checkbox
        checked={value.registration_review}
        description={t('consoleSettings.registrationReviewHint')}
        onChange={(e) => onChange({ ...value, registration_review: e.target.checked })}
      >
        {t('consoleSettings.registrationReview')}
      </Checkbox>
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

function SecurityVerification({ siteKey, onToken, retry }: { siteKey: string; onToken: (token: string) => void; retry: () => void }) {
  const { t } = useTranslation()
  const [failed, setFailed] = useState(false)
  const handleToken = useCallback((token: string) => {
    onToken(token)
    if (token) setFailed(false)
  }, [onToken])
  const onError = useCallback(() => setFailed(true), [])
  return <div className="space-y-2">
    <Notice tone="info">{t('consoleSettings.turnstileProof')}</Notice>
    <Turnstile siteKey={siteKey} action="settings_security" onToken={handleToken} onError={onError} />
    {failed && <Button onClick={retry}>{t('polish.retryVerification')}</Button>}
  </div>
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
            onValueChange={(next) => onChange({ ...value, float_direction: next })}
          >
            <SelectOption value="up">{t('consoleSettings.up')}</SelectOption>
            <SelectOption value="down">{t('consoleSettings.down')}</SelectOption>
          </Select>
        </Field>
      </FieldGrid>
    </div>
  )
}

export function RiskForm({ value, onChange, errorText }: {
  value: SettingsDrafts['risk']
  onChange: (patch: Partial<SettingsDrafts['risk']>) => void
  errorText: (field: string) => string | undefined
}) {
  const { t } = useTranslation()
  const error = errorText('checkout_turnstile_enabled')
  return <>
    <Field htmlFor="risk-ip-limit" label={t('consoleSettings.riskIpLimit')} hint={t('consoleSettings.riskIpLimitHint')} error={errorText('ip_daily_order_limit')}>
      <Input id="risk-ip-limit" inputMode="numeric" aria-describedby="risk-ip-limit-hint" value={value.ip_daily_order_limit} onChange={(event) => onChange({ ip_daily_order_limit: event.target.value })} />
    </Field>
    <Field htmlFor="risk-ip-blacklist" label={t('consoleSettings.riskIpBlacklist')} hint={t('consoleSettings.riskIpBlacklistHint')} error={errorText('ip_blacklist')}>
      <Textarea id="risk-ip-blacklist" rows={5} aria-describedby="risk-ip-blacklist-hint" value={value.ip_blacklist} onChange={(event) => onChange({ ip_blacklist: event.target.value })} />
    </Field>
    <Field htmlFor="risk-name-words" label={t('consoleSettings.riskNameWords')} hint={t('consoleSettings.riskNameWordsHint')} error={errorText('name_block_words')}>
      <Textarea id="risk-name-words" rows={5} aria-describedby="risk-name-words-hint" value={value.name_block_words} onChange={(event) => onChange({ name_block_words: event.target.value })} />
    </Field>
    <Checkbox checked={value.checkout_turnstile_enabled} aria-invalid={!!error} aria-describedby={error ? 'risk-turnstile-error' : undefined} onChange={(event) => onChange({ checkout_turnstile_enabled: event.target.checked })}>{t('consoleSettings.riskTurnstile')}</Checkbox>
    {error && <FormError id="risk-turnstile-error">{error}</FormError>}
  </>
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
