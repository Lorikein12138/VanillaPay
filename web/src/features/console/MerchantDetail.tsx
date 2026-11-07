import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi } from '@/api/client'
import { StatusDot } from '@/components/StatusDot'
import { Button } from '@/components/ui/button'
import { DetailList } from '@/components/ui/detail-list'
import { BackLink, Card, EmptyState, Page, PageHeader, SectionHeader } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { RecordLink, RecordResults } from '@/components/ui/records'
import { OrderTime } from '@/features/merchant/OrderParts'
import { money } from '@/features/merchant/overview-data'
import { Channels, DeviceStatus, MerchantStatus } from './ManageParts'
import { MerchantDialog, type MerchantAction } from './MerchantDialog'
import { planName } from './manage-data'

export function MerchantDetail({ id }: { id: string }) {
  const { t, i18n } = useTranslation(), [mode, setMode] = useState<MerchantAction | null>(null), [notice, setNotice] = useState('')
  const query = useQuery({ queryKey: ['console', 'merchant', id], queryFn: ({ signal }) => consoleApi.merchant(id, signal) })
  const detail = query.data
  if (query.error instanceof APIError && query.error.status === 404) return <EmptyState title={t('manage.notFound')} />
  return <Page>
    <PageHeader title={detail?.user.username ?? t('manage.merchant')}
      back={<BackLink to="/console/merchants">{t('manage.backMerchants')}</BackLink>}
      actions={detail && <>
        <Button onClick={() => setMode('grant')}>{t('manage.adjust')}</Button>
        <Button onClick={() => setMode('reset')}>{t('manage.reset')}</Button>
        <Button variant={detail.user.banned ? 'secondary' : 'dangerOutline'} onClick={() => setMode(detail.user.banned ? 'unban' : 'ban')}>
          {t(detail.user.banned ? 'manage.unban' : 'manage.ban')}
        </Button>
      </>} />
    {notice && <Notice tone="success" onDismiss={() => setNotice('')}>{notice}</Notice>}
    <RecordResults busy={query.isPending} error={query.error} retry={() => void query.refetch()}>
      {detail && <>
        <Card><SectionHeader title={t('manage.basic')} /><DetailList columns={2} items={[
          { label: t('manage.username'), value: detail.user.username },
          { label: t('manage.email'), value: detail.user.email },
          { label: 'PID', value: detail.user.pid, mono: true },
          { label: t('manage.registered'), value: <OrderTime value={detail.user.created_at} /> },
          { label: t('manage.twoFactor'), value: t(detail.user.totp_enabled ? 'manage.enabled' : 'manage.disabled') },
          { label: t('manage.status'), value: <MerchantStatus banned={detail.user.banned} /> },
          { label: t('manage.banTime'), value: detail.user.banned_at && <OrderTime value={detail.user.banned_at} /> },
          { label: t('manage.banReason'), value: detail.user.ban_reason },
        ]} /></Card>
        <Card><SectionHeader title={t('manage.subscription')} /><DetailList columns={2} items={[
          { label: t('manage.plan'), value: planName(detail.subscription.plan, i18n.language) || t('manage.none') },
          { label: t('manage.channels'), value: <Channels value={detail.subscription.channels} /> },
          { label: t('manage.expiry'), value: <OrderTime value={detail.subscription.expires_at} /> },
          { label: t('manage.source'), value: t('manage.sources.' + detail.subscription.source, { defaultValue: detail.subscription.source }) },
          { label: t('manage.status'), value: <StatusDot tone={detail.subscription.active ? 'ok' : 'off'}>{t(detail.subscription.active ? 'manage.active' : 'manage.inactive')}</StatusDot> },
        ]} /><RecordLink to={'/console/purchases?merchant_id=' + id}>{t('manage.purchasesLink')}</RecordLink></Card>
        <Card><SectionHeader title={t('manage.device')} /><DetailList columns={2} items={[
          { label: t('manage.status'), value: <DeviceStatus device={detail.device} /> },
          { label: t('manage.deviceName'), value: detail.device.name ?? '—' },
          { label: t('manage.version'), value: detail.device.app_version ?? '—' },
          { label: t('manage.heartbeat'), value: <OrderTime value={detail.device.last_seen_at} /> },
        ]} /></Card>
        <Card><SectionHeader title={t('manage.collection')} /><DetailList columns={2} items={[
          { label: t('manage.ttl'), value: detail.collection.effective.order_ttl_seconds },
          { label: t('manage.float'), value: money(detail.collection.effective.float_limit_cents, i18n.language) },
          { label: t('manage.direction'), value: t('manage.' + detail.collection.effective.float_direction, { defaultValue: detail.collection.effective.float_direction }) },
        ]} /></Card>
      </>}
    </RecordResults>
    {mode && detail && <MerchantDialog mode={mode} detail={detail}
      onClose={() => setMode(null)} onSuccess={(message) => { setNotice(message); setMode(null) }} />}
  </Page>
}
