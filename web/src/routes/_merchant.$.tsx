import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ErrorState } from '@/components/ErrorState'

// Unimplemented merchant URLs still pass the layout guard. They show the usual
// 404 after login, without advertising pages that this slice does not implement.
export const Route = createFileRoute('/_merchant/$')({ component: MissingMerchantPage })
function MissingMerchantPage() {
  const { t } = useTranslation()
  return <ErrorState code={t('notFound.code')} title={t('notFound.title')} body={t('notFound.body')} />
}
