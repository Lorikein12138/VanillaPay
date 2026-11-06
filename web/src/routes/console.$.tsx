import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { EmptyState } from '@/components/ui/layout'

export const Route = createFileRoute('/console/$')({ component: Missing })
function Missing() { const { t } = useTranslation(); return <EmptyState title={t('notFound.title')}>{t('notFound.body')}</EmptyState> }
