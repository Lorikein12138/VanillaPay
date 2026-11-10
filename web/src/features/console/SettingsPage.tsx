import { Select, SelectOption } from '@/components/ui/select'
import { useState } from 'react'
import { useBlocker } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/input'
import { Dialog } from '@/components/ui/dialog'
import { FormActions } from '@/components/ui/field'
import { Notice } from '@/components/ui/notice'
import { Card, Page, PageHeader, SectionHeader } from '@/components/ui/layout'
import { AccountError } from '@/features/auth/account-shared'
import { settingsGroups, type SettingsGroup } from './settings-data'
import { SettingsForms } from './settings/SettingsForms'
import { LatestSettings } from './settings/LatestSettings'
import { MailTestCard } from './settings/MailForm'
import { useSettingsEditor } from './settings/useSettingsEditor'

const label = (group: SettingsGroup) => (group === 'footer_legal' ? 'footer' : group)

export function ConsoleSettingsPage({
  group,
  onGroupChange,
}: {
  group: SettingsGroup
  onGroupChange: (group: SettingsGroup) => void
}) {
  const { t } = useTranslation()
  const editor = useSettingsEditor(group)
  const { settings, drafts, busy, conflict, saved } = editor
  const [uploading, setUploading] = useState(false)
  const dirty = settingsGroups.some((key) => drafts[key])
  useBlocker({
    shouldBlockFn: ({ current, next }) =>
      busy && current.pathname === '/console/settings' && next.pathname === current.pathname,
    enableBeforeUnload: false,
  })
  const blocker = useBlocker({
    shouldBlockFn: ({ next }) => next.pathname !== '/console/settings' && dirty,
    enableBeforeUnload: dirty,
    withResolver: true,
  })

  function selectGroup(next: SettingsGroup) {
    if (busy || next === group) return
    editor.clearErrors()
    onGroupChange(next)
  }

  if (!settings) {
    return (
      <Page>
        <PageHeader title={t('console.nav.settings')} />
        {editor.queryError ? <AccountError error={editor.queryError} /> : <Card>{t('auth.loading')}</Card>}
      </Page>
    )
  }
  const groupLabel = (key: SettingsGroup) => t(`consoleSettings.groups.${label(key)}`)
  return (
    <Page>
      <PageHeader title={t('console.nav.settings')} description={t('consoleSettings.description')} />
      {saved && (
        <Notice tone="success" onDismiss={editor.dismissSaved}>
          {saved.group === 'payment' && saved.affected > 0
            ? t('consoleSettings.paymentSavedAffected', { count: saved.affected })
            : t(`consoleSettings.saved.${saved.group}`)}
        </Notice>
      )}
      <div className="grid min-w-0 gap-5 lg:grid-cols-[220px_minmax(0,1fr)]">
        <nav aria-label={t('consoleSettings.groupsLabel')} className="hidden min-w-0 flex-col gap-1 lg:flex">
          {settingsGroups.map((key) => (
            <button
              key={key}
              type="button"
              disabled={busy}
              onClick={() => selectGroup(key)}
              className={`flex min-h-11 items-center justify-between rounded-control px-3 text-left text-label disabled:opacity-50 ${
                group === key ? 'bg-hover font-medium text-text' : 'text-text-2 hover:bg-hover'
              }`}
            >
              <span>{groupLabel(key)}</span>
              {drafts[key] && (
                <>
                  {' '}
                  <span className="text-caption text-dot-wait">{t('consoleSettings.unsaved')}</span>
                </>
              )}
            </button>
          ))}
        </nav>
        <div className="flex min-w-0 flex-col gap-5">
          <div className="lg:hidden">
            <Select
              aria-label={t('consoleSettings.groupsLabel')}
              value={group}
              disabled={busy}
              onValueChange={(next) => selectGroup(next as SettingsGroup)}
            >
              {settingsGroups.map((key) => (
                <SelectOption key={key} value={key}>
                  {groupLabel(key)}
                  {drafts[key] ? ` · ${t('consoleSettings.unsaved')}` : ''}
                </SelectOption>
              ))}
            </Select>
          </div>
          <Card>
            <SectionHeader title={groupLabel(group)} />
            {editor.refreshFailed && (
              <Notice tone="error">
                {t('consoleSettings.refreshFailed')}{' '}
                <Button size="sm" disabled={busy} onClick={() => void editor.retryRefresh()}>
                  {t('consoleSettings.retryRefresh')}
                </Button>
              </Notice>
            )}
            <fieldset disabled={busy} className="m-0 min-w-0 space-y-4 border-0 p-0">
              {conflict && (
                <>
                  <Notice tone="warning">{t('consoleSettings.versionConflict')}</Notice>
                  <LatestSettings group={group} settings={settings} />
                  <Checkbox checked={false} onChange={(event) => event.target.checked && editor.acknowledge()}>
                    {t('consoleSettings.reviewLatest')}
                  </Checkbox>
                </>
              )}
              {Boolean(editor.error) && <AccountError error={editor.error} />}
              {group === 'footer_legal' && editor.fieldErrors.footer_legal && (
                <Notice tone="error">{t('consoleSettings.errors.footer_url')}</Notice>
              )}
              {editor.oversize && <Notice tone="error">{t('consoleSettings.requestTooLarge')}</Notice>}
              <SettingsForms
                key={group}
                group={group}
                settings={settings}
                editor={editor}
                setUploading={setUploading}
              />
              <FormActions className="mt-6">
                <Button
                  variant="primary"
                  loading={busy}
                  disabled={busy || conflict || editor.refreshFailed || (group === 'site' && uploading) || (editor.canVerify && !editor.proofToken)}
                  onClick={() => void editor.save()}
                >
                  {t(`consoleSettings.save.${label(group)}`)}
                </Button>
              </FormActions>
            </fieldset>
          </Card>
          {group === 'mail' && <MailTestCard dirty={!!drafts.mail} busy={busy} />}
        </div>
      </div>
      {blocker.status === 'blocked' && (
        <Dialog
          title={t('consoleSettings.leaveTitle')}
          onClose={blocker.reset}
          footer={
            <>
              <Button onClick={blocker.reset}>{t('consoleSettings.stay')}</Button>
              <Button variant="dangerOutline" onClick={blocker.proceed}>
                {t('consoleSettings.leave')}
              </Button>
            </>
          }
        >
          <p className="m-0 text-label text-text-2">{t('consoleSettings.leaveDescription')}</p>
        </Dialog>
      )}
    </Page>
  )
}
