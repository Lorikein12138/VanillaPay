import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Trans, useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/input'
import { Dialog } from '@/components/ui/dialog'
import { Notice } from '@/components/ui/notice'
import { AccountField } from '@/features/auth/account-shared'

export type SecretProps = {
  secret?: string | null
  onSecret: (value: string | null) => void
}

export function SecretField({
  label,
  configured,
  secret,
  onSecret,
  allowClear = true,
  error,
  turnstile = false,
}: SecretProps & {
  label: string
  configured: boolean
  allowClear?: boolean
  error?: string
  turnstile?: boolean
}) {
  const { t } = useTranslation()
  const [confirm, setConfirm] = useState(false)
  return (
    <>
      <AccountField
        label={label}
        type="password"
        autoComplete="new-password"
        value={secret ?? ''}
        hint={t(configured ? 'consoleSettings.secretConfigured' : 'consoleSettings.secretNotConfigured')}
        error={error}
        onChange={(event) => onSecret(event.target.value)}
      />
      {configured && allowClear && (
        <Checkbox
          checked={secret === null}
          onChange={(event) => (event.target.checked ? setConfirm(true) : onSecret(''))}
        >
          {t('consoleSettings.clearSecret')}
        </Checkbox>
      )}
      {confirm && (
        <Dialog
          title={t('consoleSettings.clearSecret')}
          onClose={() => setConfirm(false)}
          footer={
            <>
              <Button onClick={() => setConfirm(false)}>{t('manage.cancel')}</Button>
              <Button
                variant="dangerOutline"
                onClick={() => {
                  onSecret(null)
                  setConfirm(false)
                }}
              >
                {t('consoleSettings.confirmClear')}
              </Button>
            </>
          }
        >
          <Notice tone="warning">
            {turnstile ? (
              <Trans
                i18nKey="consoleSettings.clearTurnstileWarning"
                components={{
                  recovery: <Link to="/console/recovery" className="text-accent-text underline" />,
                }}
              />
            ) : (
              t('consoleSettings.clearSecretWarning')
            )}
          </Notice>
        </Dialog>
      )}
    </>
  )
}
