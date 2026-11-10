import { plansQuery } from '@/app/portal-queries'
import { DateTimeField } from '@/components/ui/date-time-picker'
import { localDateInput, parseLocalDateTime } from '@/lib/date-time'
import { SelectOption } from '@/components/ui/select'
import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi } from '@/api/client'
import { FormStack } from '@/components/ui/auth-screen'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { FormActions } from '@/components/ui/field'
import { Notice } from '@/components/ui/notice'
import { AccountError, AccountField, validPassword } from '@/features/auth/account-shared'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { ChannelChoices, Choice, Reason } from './ManageParts'
import { invalidateConsole, planName, validBanReason, validGrantReason, type Schema } from './manage-data'

export type MerchantAction = 'ban' | 'unban' | 'reset' | 'grant'
export function MerchantDialog({ mode, detail, onClose, onSuccess }: {
  mode: MerchantAction; detail: Schema['ConsoleMerchantDetail']; onClose: () => void; onSuccess: (message: string) => void
}) {
  const { t, i18n } = useTranslation(), action = useLocalAction(), client = useQueryClient()
  const [reason, setReason] = useState(''), [password, setPassword] = useState(''), [confirmation, setConfirmation] = useState('')
  const [planId, setPlanId] = useState(''), [selected, setSelected] = useState<string[]>([])
  const [expires, setExpires] = useState(localDateInput(detail.subscription.expires_at ?? undefined))
  const [expiryError, setExpiryError] = useState(false)
  const [conflict, setConflict] = useState<'subscriptionConflict' | 'planUnavailable' | null>(null)
  const plans = useQuery({ ...plansQuery(false), enabled: mode === 'grant' })
  const plan = plans.data?.items?.find((item) => item.id === planId && !item.deleted)
  const expiry = parseLocalDateTime(expires)
  const valid = mode === 'unban' || (mode === 'ban' ? validBanReason(reason) :
    mode === 'reset' ? validPassword(password, confirmation) :
      !!plan && !!selected.length && selected.every((channel) => plan.channels?.includes(channel)) &&
      validGrantReason(reason) && Number.isFinite(expiry))
  const title = t(mode === 'grant' ? 'manage.adjust' : mode === 'ban' ? 'manage.ban' : mode === 'unban' ? 'manage.unban' : 'manage.reset')
  async function submit() {
    if (!valid) return
    if (mode === 'grant' && expiry <= Date.now()) {
      setExpiryError(true)
      return
    }
    await action.run(async () => {
      try {
        let message = t('manage.granted')
        if (mode === 'ban') {
          const result = await consoleApi.banMerchant(detail.user.id, { reason: reason.trim() })
          message = t('manage.banResult', { count: result.expired_orders })
        } else if (mode === 'unban') {
          await consoleApi.unbanMerchant(detail.user.id); message = t('manage.unbanned')
        } else if (mode === 'reset') {
          await consoleApi.resetMerchantPassword(detail.user.id, { password, password_confirmation: confirmation })
          message = t('manage.resetDone')
        } else {
          await consoleApi.grantSubscription(detail.user.id, {
            plan_id: planId, expires_at: new Date(expiry).toISOString(), channels: selected,
            expected_revision: detail.subscription.revision, reason: reason.trim(),
          })
        }
        await invalidateConsole(client)
        if (!action.active.current) return
        setPassword(''); setConfirmation('')
        if (action.active.current) onSuccess(message)
      } catch (error) {
        if (!action.active.current) return
        if (mode === 'grant' && error instanceof APIError && error.problem?.code === 'VERSION_CONFLICT') {
          setConflict('subscriptionConflict'); await invalidateConsole(client)
        } else if (mode === 'grant' && error instanceof APIError && error.problem?.code === 'PLAN_UNAVAILABLE') {
          setConflict('planUnavailable'); await plans.refetch()
        } else throw error
      }
    })
  }
  return <Dialog title={title} description={mode === 'ban' ? t('manage.banDescription') : mode === 'unban' ? t('manage.unbanDescription') : undefined}
    onClose={onClose} busy={action.busy}>
    <FormStack onSubmit={(event) => { event.preventDefault(); void submit() }}>
      {conflict && <Notice tone="warning">{t(`manage.${conflict}`)}</Notice>}
      {mode === 'grant' && <>
        {plans.error != null && <AccountError error={plans.error} />}
        <Choice label={t('manage.plan')} value={planId} disabled={action.busy} onChange={(id) => {
          setPlanId(id); setSelected(plans.data?.items?.find((item) => item.id === id)?.channels ?? [])
        }}>
          <SelectOption value="">{t('manage.plan')}</SelectOption>
          {plans.data?.items?.filter((item) => !item.deleted).map((item) => <SelectOption key={item.id} value={item.id}>{planName(item, i18n.language)}</SelectOption>)}
        </Choice>
        <ChannelChoices allowed={plan?.channels ?? []} value={selected} onChange={setSelected} disabled={action.busy} />
        <DateTimeField label={t('manage.expiry')} value={expires} disabled={action.busy}
          error={expiryError && t('manage.expiryFuture')} onValueChange={(next) => { setExpires(next); setExpiryError(false) }} />
      </>}
      {(mode === 'ban' || mode === 'grant') && <Reason value={reason} onChange={setReason} disabled={action.busy}
        hint={t(mode === 'ban' ? 'manage.banHint' : 'manage.grantHint')} />}
      {mode === 'reset' && <>
        <AccountField label={t('manage.newPassword')} hint={t('manage.passwordHint')} type="password" autoComplete="new-password" value={password}
          disabled={action.busy} onChange={(event) => setPassword(event.target.value)} />
        <AccountField label={t('manage.confirmation')} error={!!confirmation && confirmation !== password && t('manage.passwordMismatch')} type="password" autoComplete="new-password" value={confirmation}
          disabled={action.busy} onChange={(event) => setConfirmation(event.target.value)} />
      </>}
      {action.error != null && <AccountError error={action.error} />}
      <FormActions><Button disabled={action.busy} onClick={onClose}>{t('manage.cancel')}</Button>
        <Button type="submit" loading={action.busy} disabled={!valid} variant={mode === 'ban' ? 'dangerOutline' : 'primary'}>{title}</Button></FormActions>
    </FormStack>
  </Dialog>
}
