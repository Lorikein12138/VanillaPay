import { ChevronDown } from 'lucide-react'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi } from '@/api/client'
import type { components } from '@/api/schema'
import { FormStack } from '@/components/ui/auth-screen'
import { Button } from '@/components/ui/button'
import { DetailList } from '@/components/ui/detail-list'
import { Dialog } from '@/components/ui/dialog'
import { Field, FieldGrid } from '@/components/ui/field'
import { Checkbox, Input, Textarea } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import { AccountError } from '@/features/auth/account-shared'
import { useLocalAction } from '@/features/merchant/CollectionParts'
import { localized } from '@/features/merchant/subscription-data'
import { cn } from '@/lib/utils'
import { bytes } from './manage-data'

type Announcement = components['schemas']['PortalAnnouncementDTO']
type Draft = Pick<components['schemas']['PortalAnnouncementCommand'], 'title' | 'body' | 'popup'>
export type AnnouncementEditor = { mode: 'create' } | { mode: 'edit' | 'delete'; item: Announcement }
export type AnnouncementOutcome = 'published' | 'saved' | 'deleted'
type Conflict = 'needsRefresh' | 'refreshed' | 'missing'

function initialDraft(item?: Announcement): Draft {
  return {
    title: item?.title ?? { zh_CN: '', en: '' },
    body: item?.body ?? { zh_CN: '', en: '' },
    popup: item?.popup ?? false,
  }
}

export function AnnouncementDialog({
  editor,
  onClose,
  refresh,
  onWrite,
  onSuccess,
}: {
  editor: AnnouncementEditor
  onClose: () => void
  refresh: (id: string) => Promise<Announcement | null>
  onWrite: () => Promise<void>
  onSuccess: (outcome: AnnouncementOutcome) => void
}) {
  const { t, i18n } = useTranslation()
  const form = useId()
  const action = useLocalAction()
  const [current, setCurrent] = useState(editor.mode === 'create' ? undefined : editor.item)
  const [draft, setDraft] = useState(() => initialDraft(current))
  const [conflict, setConflict] = useState<Conflict>()
  const [reviewedRevision, setReviewedRevision] = useState<number>()
  const [preview, setPreview] = useState(false)
  const previewId = useId()
  const deleting = editor.mode === 'delete'
  const title = t(`consoleAnnouncements.${editor.mode}`)
  const submitLabel = t(
    deleting ? 'manage.remove' : current ? 'consoleAnnouncements.save' : 'consoleAnnouncements.publish',
  )
  const fields = [
    { key: 'title', lang: 'zh_CN', label: 'zhTitle' },
    { key: 'title', lang: 'en', label: 'enTitle' },
    { key: 'body', lang: 'zh_CN', label: 'zhBody' },
    { key: 'body', lang: 'en', label: 'enBody' },
  ] as const
  function fieldError({ key, lang }: (typeof fields)[number]) {
    const value = draft[key][lang]
    if (bytes(value) > (key === 'title' ? 300 : 100000)) return t(`consoleAnnouncements.${key}TooLong`)
    if (lang === 'zh_CN' && !value.trim()) return t(`consoleAnnouncements.${key}Required`)
    return undefined
  }
  const reviewed = reviewedRevision !== undefined && reviewedRevision === current?.revision
  const valid =
    (deleting || fields.every((field) => !fieldError(field))) && (!conflict || (conflict === 'refreshed' && reviewed))

  async function loadLatest(id: string) {
    const latest = await refresh(id)
    if (!action.active.current) return
    setConflict(latest ? 'refreshed' : 'missing')
    if (latest) {
      setCurrent(latest)
      setDraft(initialDraft(latest))
      setPreview(false)
    }
  }

  function submit() {
    if (!valid) return
    void action.run(async () => {
      try {
        if (deleting && current) await consoleApi.deleteAnnouncement(current.id, { revision: current.revision })
        else if (current) await consoleApi.updateAnnouncement(current.id, { ...draft, revision: current.revision })
        else await consoleApi.publishAnnouncement(draft)
      } catch (error) {
        if (
          !current ||
          !(error instanceof APIError) ||
          error.status !== 409 ||
          error.problem?.code !== 'VERSION_CONFLICT'
        )
          throw error
        if (action.active.current) {
          setConflict('needsRefresh')
          setReviewedRevision(undefined)
          await loadLatest(current.id)
        }
        return
      }
      await onWrite()
      if (action.active.current) onSuccess(deleting ? 'deleted' : current ? 'saved' : 'published')
    })
  }

  function field(spec: (typeof fields)[number]) {
    const { key, lang, label } = spec
    const id = `${form}-${label}`
    const value = draft[key][lang]
    const error = fieldError(spec)
    const props = {
      id,
      value,
      'aria-required': lang === 'zh_CN',
      'aria-invalid': !!error && !!value,
      'aria-describedby': error && value ? `${id}-hint` : undefined,
      onChange: (event: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const next = event.target.value
        setDraft((previous) => ({ ...previous, [key]: { ...previous[key], [lang]: next } }))
      },
    }
    return (
      <Field
        key={label}
        label={t(`consoleAnnouncements.${label}`)}
        htmlFor={id}
        optional={lang === 'en' ? t('ui.optional') : undefined}
        error={value ? error : undefined}
      >
        {key === 'title' ? <Input {...props} /> : <Textarea {...props} rows={3} />}
      </Field>
    )
  }

  return (
    <Dialog
      title={title}
      description={deleting ? t('consoleAnnouncements.deleteDescription') : undefined}
      size={deleting ? 'md' : 'lg'}
      onClose={onClose}
      busy={action.busy}
      footer={
        <>
          <Button onClick={onClose} disabled={action.busy}>
            {t('manage.cancel')}
          </Button>
          <Button
            type="submit"
            form={form}
            loading={action.busy}
            disabled={!valid}
            variant={deleting ? 'dangerOutline' : 'primary'}
          >
            {submitLabel}
          </Button>
        </>
      }
    >
      <FormStack
        id={form}
        onSubmit={(event) => {
          event.preventDefault()
          submit()
        }}
      >
        {conflict === 'refreshed' && (
          <Notice tone="warning">
            {t('consoleAnnouncements.conflict')}
            {!deleting && <div>{t('consoleAnnouncements.conflictDraft')}</div>}
          </Notice>
        )}
        {conflict === 'missing' && <Notice tone="warning">{t('consoleAnnouncements.conflictMissing')}</Notice>}
        {conflict === 'needsRefresh' && (
          <Notice
            tone="warning"
            action={
              <Button disabled={action.busy} onClick={() => current && void action.run(() => loadLatest(current.id))}>
                {t('error.retry')}
              </Button>
            }
          >
            {t('consoleAnnouncements.conflictRefresh')}
          </Notice>
        )}
        {deleting && (
          conflict === 'refreshed' && current ? (
            <DetailList
              columns={2}
              items={[
                { label: t('consoleAnnouncements.revision'), value: <span className="tabular-nums">{current.revision}</span> },
                { label: t('consoleAnnouncements.popup'), value: t(current.popup ? 'polish.enabled' : 'polish.notEnabled') },
                ...fields.map(({ key, lang, label }) => ({
                  label: t(`consoleAnnouncements.${label}`),
                  value: current[key][lang] || '—',
                  wide: key === 'body',
                })),
              ]}
            />
          ) : (
            <DetailList items={[{ label: t('consoleAnnouncements.zhTitle'), value: current?.title.zh_CN }]} />
          )
        )}
        {conflict === 'refreshed' && (
          <Checkbox
            checked={reviewed}
            disabled={action.busy}
            onChange={(event) => setReviewedRevision(event.target.checked ? current?.revision : undefined)}
          >
            {t('consoleAnnouncements.reviewed')}
          </Checkbox>
        )}
        {!deleting && (
          <fieldset
            disabled={action.busy || conflict === 'needsRefresh' || conflict === 'missing'}
            className="m-0 min-w-0 space-y-4 border-0 p-0"
          >
            {current && draft.popup && <Notice tone="warning">{t('consoleAnnouncements.popupAgain')}</Notice>}
            <FieldGrid className="grid-cols-1 sm:grid-cols-2">{fields.slice(0, 2).map(field)}</FieldGrid>
            {fields.slice(2).map(field)}
            <Checkbox checked={draft.popup} onChange={(event) => setDraft({ ...draft, popup: event.target.checked })}>
              {t('consoleAnnouncements.popup')}
            </Checkbox>
            <div>
              <Button size="sm" aria-expanded={preview} aria-controls={previewId} onClick={() => setPreview(!preview)}>
                {t('consoleAnnouncements.preview')}
                <ChevronDown aria-hidden="true" className={cn('transition-transform duration-150 motion-reduce:transition-none', preview && 'rotate-180')} />
              </Button>
              {preview && (
                <DetailList
                  id={previewId}
                  className="mt-3"
                  items={[
                    {
                      wide: true,
                      label:
                        localized(draft.title, i18n.language) || draft.title.zh_CN || t('consoleAnnouncements.preview'),
                      value: localized(draft.body, i18n.language) || draft.body.zh_CN || '—',
                    },
                  ]}
                />
              )}
            </div>
          </fieldset>
        )}
        {action.error != null && <AccountError error={action.error} />}
      </FormStack>
    </Dialog>
  )
}
