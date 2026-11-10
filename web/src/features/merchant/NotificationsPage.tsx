import { notificationsQuery } from '@/app/portal-queries'
import { useResetPortalCursors } from '@/app/portal-cursors'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api/client'
import type { components } from '@/api/schema'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, EmptyState, Page, PageHeader, Skeleton } from '@/components/ui/layout'
import { List, ListRow } from '@/components/ui/list'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { ModuleError, useLocalAction } from './CollectionParts'
import { OrderTime } from './OrderParts'
import { invalidateNotifications, notificationTarget } from './notification-data'
import { localized } from './subscription-data'

export function NotificationsPage({ name }: { name: string }) {
  const { t, i18n } = useTranslation(), client = useQueryClient(), navigate = useNavigate()
  const [unread, setUnread] = useState(false), action = useLocalAction()
  const options = notificationsQuery(name, unread)
  const query = useInfiniteQuery(options)
  useResetPortalCursors(options.queryKey)
  const items = query.data?.pages.flatMap((page) => page.items ?? []) ?? []
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  async function read(value: components['schemas']['PortalReadRequest'], target?: string | null) {
    await action.run(async () => {
      await api.readNotifications(value)
      await invalidateNotifications(client, name)
      if (action.active.current && target) await navigate({ to: target })
    })
  }
  return <Page>
    <PageHeader title={t('merchant.nav.notifications')} description={t('notifications.description')} actions={<Button disabled={action.busy || query.isPlaceholderData || !query.data?.pages[0]?.unread_count} loading={action.busy} onClick={() => { void read({ all: true }) }}>{t('notifications.readAll')}</Button>} />
    <SegmentedControl label={t('notifications.filter')} value={unread ? 'unread' : 'all'} options={[{ value: 'all', label: t('notifications.all') }, { value: 'unread', label: t('notifications.unread') }]} onValueChange={(value) => setUnread(value === 'unread')} />
    {action.error != null ? <ModuleError error={action.error} /> : query.isError && <ModuleError error={query.error} retry={() => { void (query.isFetchNextPageError ? query.fetchNextPage() : query.refetch()) }} />}
    {query.isPending && <Skeleton className="h-64" />}
    {query.data && (items.length ? <Card><List label={t('merchant.nav.notifications')}>{items.map((item) => {
      const target = notificationTarget(item.action)
      return <ListRow key={item.id} label={localized(item.title, language)} title={localized(item.title, language)} unread={!item.read_at} description={localized(item.summary, language)} meta={<>{t(item.read_at ? 'notifications.read' : 'notifications.unread')} · <OrderTime value={item.created_at} /></>} actions={<>
        {!item.read_at && <Button variant="ghost" size="sm" disabled={action.busy} onClick={() => { void read({ ids: [item.id] }) }}>{t('notifications.markRead')}</Button>}
        {target && <a href={target} aria-disabled={action.busy || undefined} className={buttonVariants({ variant: 'link', size: 'sm' })} onClick={(event) => { event.preventDefault(); if (!action.busy) void read({ ids: [item.id] }, target) }}>{t('notifications.view')}</a>}
      </>} />
    })}</List></Card> : <EmptyState title={t(unread ? 'notifications.emptyUnread' : 'notifications.empty')} />)}
    {query.hasNextPage && !query.isPlaceholderData && <div className="flex justify-center"><Button loading={query.isFetchingNextPage} onClick={() => { void query.fetchNextPage() }}>{t('polish.loadMore')}</Button></div>}
  </Page>
}
