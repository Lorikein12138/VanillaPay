import { useTranslation } from 'react-i18next'
import { type Settings, type SettingsGroup } from '../settings-data'
import { FooterForm, HomeForm } from './ContentForms'
import { MailForm } from './MailForm'
import { BillingForm, PaymentForm, SecurityForm } from './PolicyForms'
import { SiteForm } from './SiteForm'
import type { useSettingsEditor } from './useSettingsEditor'

export function SettingsForms({
  group,
  settings,
  editor,
  setUploading,
}: {
  group: SettingsGroup
  settings: Settings
  editor: ReturnType<typeof useSettingsEditor>
  setUploading: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const { fieldErrors, groupValue, update, secretInput, updateSecret } = editor
  const translate = t as (key: string) => string
  const errorText = (key: string) =>
    fieldErrors[key] ? translate(`consoleSettings.errors.${fieldErrors[key]}`) : undefined
  const secretProps = { secret: secretInput, onSecret: updateSecret }
  switch (group) {
    case 'site':
      return (
        <SiteForm
          value={groupValue(group, settings)}
          update={(patch) => update(group, patch)}
          error={errorText('site_name')}
          setUploading={setUploading}
        />
      )
    case 'home':
      return (
        <HomeForm
          value={groupValue(group, settings)}
          onChange={(value) => update(group, value)}
          errors={{ mode: errorText('mode'), html: errorText('html'), url: errorText('url') }}
        />
      )
    case 'footer_legal':
      return (
        <FooterForm
          value={groupValue(group, settings)}
          onChange={(value) => update(group, value)}
          errors={fieldErrors}
          errorText={errorText}
        />
      )
    case 'mail':
      return (
        <MailForm
          value={groupValue(group, settings)}
          onChange={(value) => update(group, value)}
          configured={settings.mail.password_configured}
          dirty={!!editor.drafts.mail}
          errorText={errorText}
          {...secretProps}
        />
      )
    case 'security':
      return (
        <SecurityForm
          value={groupValue(group, settings)}
          onChange={(value) => update(group, value)}
          configured={settings.security.secret_configured}
          errorText={errorText}
          {...secretProps}
        />
      )
    case 'payment':
      return (
        <PaymentForm
          value={groupValue(group, settings)}
          onChange={(value) => update(group, value)}
          errorText={errorText}
        />
      )
    case 'billing':
      return (
        <BillingForm
          value={groupValue(group, settings)}
          onChange={(value) => update(group, value)}
          settings={settings.billing}
          errorText={errorText}
          {...secretProps}
        />
      )
  }
}
