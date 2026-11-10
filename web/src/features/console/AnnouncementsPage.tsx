import { announcementsQuery } from '@/app/portal-queries'
import { useState } from 'react'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { components } from '@/api/schema'
import { Button } from '@/components/ui/button'
import { EmptyState, Mono, Page, PageHeader } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { InlineActions, RecordBadge, RecordResults, Records, RecordText } from '@/components/ui/records'
import { OrderTime } from '@/features/merchant/OrderParts'
import { AnnouncementDialog, type AnnouncementEditor, type AnnouncementOutcome } from './AnnouncementDialog'

type Announcement = components['schemas']['PortalAnnouncementDTO']
const options = announcementsQuery()

export function AnnouncementsPage() {
  const { t } = useTranslation()
  const client = useQueryClient()
  const [editor, setEditor] = useState<AnnouncementEditor | null>(null)
  const [notice, setNotice] = useState<AnnouncementOutcome | null>(null)
  const query = useInfiniteQuery(options)
  const items = query.data?.pages.flatMap((page) => page.items ?? []) ?? []

  async function refreshAnnouncement(id: string) {
    let result = await query.refetch({ throwOnError: true })
    // A concurrent publication can push an edited item beyond the loaded pages.
    while (true) {
      const found = result.data?.pages.flatMap((page) => page.items ?? []).find((item) => item.id === id)
      if (found || !result.data?.pages.at(-1)?.next_cursor) return found ?? null
      result = await query.fetchNextPage({ throwOnError: true })
    }
  }

  function open(next: AnnouncementEditor) {
    setNotice(null)
    setEditor(next)
  }

  const create = () => open({ mode: 'create' })
  return (
    <Page>
      <PageHeader
        title={t('consoleAnnouncements.title')}
        description={t('consoleAnnouncements.description')}
        actions={
          <Button variant="primary" onClick={create}>
            {t('consoleAnnouncements.create')}
          </Button>
        }
      />
      {notice && <Notice onDismiss={() => setNotice(null)}>{t(`consoleAnnouncements.${notice}`)}</Notice>}
      <RecordResults
        busy={query.isPending}
        error={query.error}
        retry={() => void (query.isFetchNextPageError ? query.fetchNextPage() : query.refetch())}
        total={query.data?.pages[0]?.total}
        more={query.hasNextPage}
        loadingMore={query.isFetchingNextPage}
        onMore={() => void query.fetchNextPage()}
      >
        {items.length > 0 && (
          <Records<Announcement>
            items={items}
            id={(item) => item.id}
            label={t('consoleAnnouncements.title')}
            columns={[
              {
                label: t('consoleAnnouncements.zhTitle'),
                value: (item) => <RecordText>{item.title.zh_CN}</RecordText>,
              },
              {
                label: t('consoleAnnouncements.popupColumn'),
                value: (item) => (item.popup ? <RecordBadge>{t('consoleAnnouncements.popupBadge')}</RecordBadge> : '—'),
              },
              {
                label: t('consoleAnnouncements.publishedAt'),
                value: (item) => <OrderTime value={item.published_at} />,
                nowrap: true,
              },
              { label: t('consoleAnnouncements.revision'), value: (item) => <Mono>{item.revision}</Mono> },
              {
                label: t('manage.manage'),
                wide: true,
                value: (item) => (
                  <InlineActions>
                    <Button size="sm" onClick={() => open({ mode: 'edit', item })}>
                      {t('manage.edit')}
                    </Button>
                    <Button size="sm" variant="dangerOutline" onClick={() => open({ mode: 'delete', item })}>
                      {t('manage.remove')}
                    </Button>
                  </InlineActions>
                ),
              },
            ]}
          />
        )}
        {query.data && items.length === 0 && (
          <EmptyState title={t('consoleAnnouncements.empty')}>
            <Button variant="link" onClick={create}>
              {t('consoleAnnouncements.create')}
            </Button>
          </EmptyState>
        )}
      </RecordResults>
      {editor && (
        <AnnouncementDialog
          editor={editor}
          onClose={() => setEditor(null)}
          refresh={refreshAnnouncement}
          onWrite={() => client.invalidateQueries({ queryKey: options.queryKey })}
          onSuccess={(outcome) => {
            setEditor(null)
            setNotice(outcome)
          }}
        />
      )}
    </Page>
  )
}
