import { useTranslation } from 'react-i18next'
import { StatusDot } from '@/components/StatusDot'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import { authErrorKey } from '@/features/auth/errors'
import { useLocalAction } from './CollectionParts'
import { useMerchantAccount } from './AccountProvider'

export function RegistrationReviewPage() {
  const { t } = useTranslation()
  const account = useMerchantAccount()!
  const action = useLocalAction()
  const rejected = account.user.approval_state === 'rejected'
  return <section className="flex max-w-xl flex-col items-start gap-5">
    <StatusDot tone={rejected ? 'off' : 'wait'}>{t(rejected ? 'manage.approvalStates.rejected' : 'manage.approvalStates.pending')}</StatusDot>
    <h1 className="m-0 font-serif text-title font-normal">{t(rejected ? 'merchant.registrationReview.rejected' : 'merchant.registrationReview.pending')}</h1>
    <p className="m-0 break-words text-text-2">{rejected ? account.user.approval_reason : t('merchant.registrationReview.hint')}</p>
    {action.error != null && <Notice tone="error">{t(authErrorKey(action.error))}</Notice>}
    <Button loading={action.busy} onClick={() => { void action.run(account.refresh) }}>{t('merchant.registrationReview.refresh')}</Button>
  </section>
}
