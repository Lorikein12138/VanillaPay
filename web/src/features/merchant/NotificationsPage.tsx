import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import type { components } from '@/api/schema'
import { Button, buttonVariants } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Card, EmptyState, Page, PageHeader, Skeleton } from '@/components/ui/layout'
import { List, ListRow } from '@/components/ui/list'
import { Notice } from '@/components/ui/notice'
import { SegmentedControl } from '@/components/ui/segmented-control'
import { ModuleError, useLocalAction } from './CollectionParts'
import { OrderTime } from './OrderParts'
import { invalidateNotifications, notificationTarget } from './notification-data'
import { localized } from './subscription-data'

export function UnreadBadge({ count }: { count: number }) {
  const { t } = useTranslation()
  if (count <= 0) return null
  return <span className="ml-auto rounded-full bg-accent/10 px-2 py-0.5 font-mono text-caption text-accent-text" aria-label={t('notifications.unreadCount', { count })}>{count > 99 ? '99+' : count}</span>
}

export function NotificationsPage({ name }: { name: string }) {
  const { t, i18n } = useTranslation(), client = useQueryClient(), navigate = useNavigate()
  const [unread, setUnread] = useState(false), action = useLocalAction()
  const query = useInfiniteQuery({ queryKey: ['merchant', name, 'notifications', 'list', { unread }], queryFn: ({ pageParam }) => api.notifications({ limit: 20, cursor: pageParam, unread_only: unread }), initialPageParam: undefined as string | undefined, getNextPageParam: (page) => page.next_cursor ?? undefined, gcTime: 0 })
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
    <PageHeader title={t('merchant.nav.notifications')} description={t('notifications.description')} actions={<Button disabled={action.busy || !query.data?.pages[0]?.unread_count} loading={action.busy} onClick={() => { void read({ all: true }) }}>{t('notifications.readAll')}</Button>} />
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
    {query.hasNextPage && <div className="flex justify-center"><Button loading={query.isFetchingNextPage} onClick={() => { void query.fetchNextPage() }}>{t('polish.loadMore')}</Button></div>}
  </Page>
}

export function PopupAnnouncements({ name, items }: { name: string; items: components['schemas']['PortalAnnouncementDTO'][] }) {
  const [skipped, setSkipped] = useState<Set<string>>(() => new Set())
  const current = items.find((item) => item.popup && !item.read && !skipped.has(`${item.id}:${item.revision}`))
  if (!current) return null
  return <AnnouncementDialog key={`${current.id}:${current.revision}`} name={name} item={current} onSkip={() => setSkipped((old) => new Set([...old, `${current.id}:${current.revision}`]))} />
}
function AnnouncementDialog({ name, item, onSkip }: { name: string; item: components['schemas']['PortalAnnouncementDTO']; onSkip: () => void }) {
  const { t, i18n } = useTranslation(), client = useQueryClient(), action = useLocalAction()
  async function acknowledge() {
    await action.run(async () => {
      try { await api.readAnnouncement(item.id, { revision: item.revision }) }
      catch (error) {
        if (!(error instanceof APIError) || (error.status !== 404 && !(error.status === 409 && error.problem?.code === 'VERSION_CONFLICT'))) throw error
      }
      // Hide this revision immediately; an updated revision remains eligible.
      if (action.active.current) onSkip()
      await invalidateNotifications(client, name)
    })
  }
  const language = i18n.resolvedLanguage ?? 'zh-CN'
  return <Dialog
    title={localized(item.title, language) || item.title.zh_CN}
    description={t('notifications.announcement')}
    busy={action.busy}
    onClose={onSkip}
    footer={<Button variant="primary" loading={action.busy} onClick={() => void acknowledge()}>{t('notifications.acknowledge')}</Button>}
  >
    <div className="whitespace-pre-wrap break-words text-label leading-relaxed text-text-2">
      {localized(item.body, language) || item.body.zh_CN}
    </div>
    <p className="mt-5 text-caption text-text-3"><OrderTime value={item.published_at} /></p>
    {action.error != null && <Notice tone="error">{t('notifications.readFailed')}</Notice>}
  </Dialog>
}
