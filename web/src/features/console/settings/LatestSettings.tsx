import { useTranslation } from 'react-i18next'
import { DetailList } from '@/components/ui/detail-list'
import { centsToYuan } from '@/features/merchant/collection-data'
import { writableGroup, type Settings, type SettingsGroup } from '../settings-data'

export function LatestSettings({ group, settings }: { group: SettingsGroup; settings: Settings }) {
  const { t } = useTranslation()
  const translate = t as (key: string) => string
  const row = (key: string, value: string | number | null | undefined) => ({
    label: translate(`consoleSettings.${key}`),
    value: value === '' || value == null ? '—' : value,
  })
  const enabled = (value: boolean) => t(value ? 'consoleSettings.enabled' : 'consoleSettings.disabled')
  const configured = (value: boolean) =>
    t(value ? 'consoleSettings.configured' : 'consoleSettings.notConfigured')
  switch (group) {
    case 'site': {
      const value = settings.site
      return (
        <DetailList columns={2} items={[row('siteName', value.site_name), row('logo', value.logo_asset_id), row('favicon', value.favicon_asset_id)]} />
      )
    }
    case 'home': {
      const value = settings.home
      return (
        <DetailList
          columns={2}
          items={[
            row('homeMode', t(`consoleSettings.homeModes.${value.mode}`)),
            ...(value.mode === 'html' ? [row('homeHtml', value.html)] : []),
            ...(value.mode === 'url' ? [row('homeUrl', value.url)] : []),
          ]}
        />
      )
    }
    case 'footer_legal': {
      const value = writableGroup(settings, group)
      const items = (value.items ?? []).flatMap((item, index) => [
        {
          label: t('consoleSettings.footerItemNumber', { number: index + 1 }),
          value: [item.label.zh_CN, item.label.en, item.url, enabled(item.enabled)].filter(Boolean).join(' · '),
          wide: true,
        },
        row('footerTextZh', item.text.zh_CN),
        row('footerTextEn', item.text.en),
      ])
      return (
        <DetailList
          columns={2}
          items={[
            ...items,
            row('terms', [value.terms.zh_CN, value.terms.en].filter(Boolean).join('\n')),
            row('privacy', [value.privacy.zh_CN, value.privacy.en].filter(Boolean).join('\n')),
          ]}
        />
      )
    }
    case 'mail': {
      const value = settings.mail
      return (
        <DetailList
          columns={2}
          items={[
            row('mailHost', value.host),
            row('mailPort', value.port),
            row('mailUsername', value.username),
            row('mailAddress', value.from_address),
            row('mailName', value.from_name),
            row('mailTls', value.tls_mode),
            row('mailPassword', configured(value.password_configured)),
          ]}
        />
      )
    }
    case 'security': {
      const value = settings.security
      return (
        <DetailList
          columns={2}
          items={[
            row('turnstileSite', value.turnstile_site_key),
            row('turnstileEnabled', t(value.turnstile_enabled ? 'consoleSettings.enabled' : 'consoleSettings.disabled')),
            row('restrictDomains', enabled(value.restrict_email_domains)),
            row('emailDomains', value.email_domains?.join(', ')),
            row('banContactZh', value.ban_contact.zh_CN),
            row('banContactEn', value.ban_contact.en),
            row('registrationReview', enabled(value.registration_review)),
            row('turnstileSecret', configured(value.secret_configured)),
          ]}
        />
      )
    }
    case 'payment': {
      const value = settings.payment
      return (
        <DetailList
          columns={2}
          items={[
            row('min', centsToYuan(value.min_amount_cents)),
            row('max', centsToYuan(value.max_amount_cents)),
            row('floatMin', centsToYuan(value.float_min_cents)),
            row('floatMax', centsToYuan(value.float_max_cents)),
            row('floatDefault', centsToYuan(value.float_default_cents)),
            row('defaultTtl', value.default_ttl_seconds),
            row('maxTtl', value.max_ttl_seconds),
            row('floatDirection', translate(`consoleSettings.${value.float_direction}`)),
          ]}
        />
      )
    }
    case 'risk': {
      const value = settings.risk
      return <DetailList columns={2} items={[
        row('riskIpLimit', value.ip_daily_order_limit),
        row('riskIpBlacklist', value.ip_blacklist.join(' · ')),
        row('riskNameWords', value.name_block_words.join(' · ')),
        row('riskTurnstile', enabled(value.checkout_turnstile_enabled)),
      ]} />
    }
    case 'billing': {
      const value = settings.billing
      return (
        <DetailList
          columns={2}
          items={[
            row('billingPid', value.merchant_pid),
            row('billingKey', configured(value.key_configured)),
            row('billingValidity', enabled(value.credential_valid)),
          ]}
        />
      )
    }
  }
}
