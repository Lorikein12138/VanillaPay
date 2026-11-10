import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import type { components } from '@/api/schema'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Notice } from '@/components/ui/notice'
import { useLocalAction } from './CollectionParts'
import { OrderTime } from './OrderParts'
import { invalidateNotifications } from './notification-data'
import { localized } from './subscription-data'

export function UnreadBadge({ count }: { count: number }) {
  const { t } = useTranslation()
  if (count <= 0) return null
  return <span className="ml-auto rounded-full bg-accent/10 px-2 py-0.5 font-mono text-caption text-accent-text" aria-label={t('notifications.unreadCount', { count })}>{count > 99 ? '99+' : count}</span>
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
