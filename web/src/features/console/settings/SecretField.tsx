import { useId, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { Trans, useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Field } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Dialog } from '@/components/ui/dialog'
import { Notice } from '@/components/ui/notice'

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
  clearUnavailableHint,
  error,
  turnstile = false,
}: SecretProps & {
  label: string
  configured: boolean
  allowClear?: boolean
  clearUnavailableHint?: string
  error?: string
  turnstile?: boolean
}) {
  const { t } = useTranslation()
  const id = useId()
  const [confirm, setConfirm] = useState(false)
  const pendingClear = secret === null
  const clearTitle = t('consoleSettings.clearSecretTitle', { label })
  const configuredHint = t(configured ? 'consoleSettings.secretConfigured' : 'consoleSettings.secretNotConfigured')
  const hint = pendingClear
    ? t('consoleSettings.secretPendingClear', { label })
    : [configuredHint, clearUnavailableHint].filter(Boolean).join(' ')
  return (
    <>
      <Field htmlFor={id} label={label} error={error}>
        <Input
          id={id}
          type="password"
          autoComplete="new-password"
          value={secret ?? ''}
          disabled={pendingClear}
          placeholder={pendingClear ? t('consoleSettings.secretPendingPlaceholder') : undefined}
          aria-invalid={!!error || undefined}
          aria-describedby={error ? `${id}-status ${id}-hint` : `${id}-status`}
          onChange={(event) => onSecret(event.target.value)}
        />
        <div className="flex min-w-0 items-center justify-between gap-2">
          <p id={`${id}-status`} className="m-0 text-caption leading-relaxed text-text-3">
            {hint}
          </p>
          {pendingClear ? (
            <Button variant="ghost" size="sm" onClick={() => onSecret('')}>
              {t('consoleSettings.undoClearSecret')}
            </Button>
          ) : configured && allowClear && (
            <Button variant="danger" size="sm" aria-label={clearTitle} onClick={() => setConfirm(true)}>
              {t('consoleSettings.clearSecretAction')}
            </Button>
          )}
        </div>
      </Field>
      {confirm && (
        <Dialog
          title={clearTitle}
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
