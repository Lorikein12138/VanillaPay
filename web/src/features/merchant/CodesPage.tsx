import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError, api } from '@/api/client'
import { StatusDot } from '@/components/StatusDot'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { DetailList } from '@/components/ui/detail-list'
import { Field, FieldGrid, FormActions } from '@/components/ui/field'
import { FileInput } from '@/components/ui/file-input'
import { Input, Select } from '@/components/ui/input'
import { Card, Page, PageHeader, SectionHeader, Skeleton } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { collectionInput, centsToYuan, invalidateCollection, uploadError, type Schema } from './collection-data'
import { ModuleError, useLocalAction } from './CollectionParts'
import { orderChannels } from './orders-data'
import { OrderChannel, OrderTime } from './OrderParts'

export function CodesPage({ name }: { name: string }) {
  const { t } = useTranslation()
  const [notice, setNotice] = useState('')
  const [noticeTone, setNoticeTone] = useState<'success' | 'error'>('success')
  function showNotice(message: string, tone: 'success' | 'error' = 'success') { setNotice(message); setNoticeTone(tone) }
  const collection = useQuery({ queryKey: ['merchant', name, 'collection'], queryFn: api.collection })
  const codes = useQuery({ queryKey: ['merchant', name, 'codes'], queryFn: api.qrSlots })
  return <Page>
    <PageHeader title={t('polish.collectionCodes')} description={t('polish.manageCollectionCodesAndPaymentSettingsFor')} />
    {notice && <Notice tone={noticeTone} onDismiss={() => setNotice('')}>{notice}</Notice>}
    {(collection.isError || codes.isError) && <ModuleError error={collection.error ?? codes.error} retry={() => { void collection.refetch(); void codes.refetch() }} />}
    <section aria-label={t('polish.collectionCodeSlots')} aria-busy={codes.isPending} className="grid gap-5 lg:grid-cols-3">
      {(codes.data || collection.data) && orderChannels.map((channel) => <QRSlot key={channel} name={name} onNotice={showNotice} channel={channel} value={(codes.data?.codes ?? collection.data?.codes)?.find((code) => code.channel === channel)} />)}
      {codes.isPending && <Skeleton className="h-96" />}
    </section>
    {collection.data && <CollectionForm key={JSON.stringify(collection.data.settings)} name={name} value={collection.data.settings} onNotice={showNotice} />}
  </Page>
}

function QRSlot({ name, channel, value, onNotice }: { name: string; channel: string; onNotice: (message: string, tone?: 'success' | 'error') => void; value?: Schema['SettingsQRDTO'] }) {
  const { t } = useTranslation()
  const client = useQueryClient(), action = useLocalAction(), id = useId()
  const [file, setFile] = useState<File | null>(null)
  const [confirm, setConfirm] = useState<'upload' | 'toggle' | null>(null)
  async function submit() {
    await action.run(async () => {
      if (confirm === 'upload' && file) {
        const bitmap = await createImageBitmap(file)
        const valid = bitmap.width <= 4096 && bitmap.height <= 4096
        bitmap.close()
        if (!valid) throw new Error(t('polish.imageWidthAndHeightMustNotExceed'))
        await api.uploadQR(channel, file)
      } else if (confirm === 'toggle') await api.setQREnabled(channel, !value?.enabled)
      await invalidateCollection(client, name)
      setConfirm(null); setFile(null); onNotice(t('polish.collectionCodeUpdated'))
    })
  }
  return <Card className="flex flex-col" aria-label={t('polish.channelCollectionCode', { v0: channel })}>
    <SectionHeader as="h2" title={<OrderChannel channel={channel} />} actions={<StatusDot tone={value?.asset_id && value.enabled ? 'ok' : 'off'}>{value?.asset_id ? value.enabled ? t('polish.active') : t('polish.inactive') : t('polish.notUploaded')}</StatusDot>} />
    <div className="mb-5 flex h-44 items-center justify-center rounded-control bg-muted p-4">{value?.image_url ? <img className="max-h-full max-w-full rounded-control" src={value.image_url} alt={t('polish.channelCodePreview', { v0: channel })} /> : <p className="text-label text-text-3">{t('polish.yourUploadedImageAppearsHere')}</p>}</div>
    <p className="mb-5 text-caption text-text-3">{t('polish.updated')}<OrderTime value={value?.updated_at ?? null} /></p>
    <div className="mt-auto flex flex-col gap-4"><Field htmlFor={id} label={value?.asset_id ? t('polish.replaceImage') : t('polish.uploadImage')} hint={t('polish.pngJpegUpTo5MibUp')}><FileInput id={id} accept="image/png,image/jpeg" disabled={action.busy} onChange={(event) => { const next = event.target.files?.[0]; event.target.value = ''; onNotice(''); if (next) { const error = uploadError(next); onNotice(error ? t(error) : '', 'error'); if (!error) { setFile(next); setConfirm('upload') } } }} /></Field>
      <FormActions><Button disabled={!value?.asset_id} onClick={() => setConfirm('toggle')}>{value?.enabled ? t('polish.disable') : t('polish.enable')}</Button></FormActions>
    </div>
    {confirm && <Dialog title={confirm === 'upload' ? t('polish.confirmCollectionCodeUpload') : t('polish.confirmStatusChange')} description={confirm === 'upload' ? t('polish.theImageReplacesThisChannelSCode') : t('polish.thisChangesTheChannelSAvailabilityFor')} busy={action.busy} onClose={() => { setConfirm(null); setFile(null) }} footer={<Button variant="primary" loading={action.busy} onClick={() => { void submit() }}>{action.busy ? t('polish.submitting') : t('polish.confirmSave')}</Button>}>
      {file && <p className="break-all text-label">{file.name}</p>}{action.error != null && <ModuleError error={action.error} />}
    </Dialog>}
  </Card>
}

function CollectionForm({ name, value, onNotice }: { name: string; value: Schema['SettingsCollectionSettings']; onNotice: (value: string) => void }) {
  const { t } = useTranslation()
  const client = useQueryClient(), action = useLocalAction(), id = useId()
  const [ttl, setTTL] = useState(value.order_ttl_seconds?.toString() ?? '')
  const [amount, setAmount] = useState(value.float_limit_cents === null ? '' : centsToYuan(value.float_limit_cents))
  const [direction, setDirection] = useState(value.float_direction ?? '')
  const [invalid, setInvalid] = useState(false)
  const [pending, setPending] = useState<Schema['SettingsSaveCollectionRequest'] | null>(null)
  async function submit() {
    if (!pending) return
    await action.run(async () => {
      try { await api.saveCollection(pending) }
      catch (error) { if (error instanceof APIError && error.status === 409) { setPending(null); onNotice(t('polish.settingsChangedAndHaveBeenReloadedReview')); await invalidateCollection(client, name) }; throw error }
      setPending(null); onNotice(t('polish.collectionSettingsSaved')); await invalidateCollection(client, name)
    })
  }
  return <Card><SectionHeader title={t('polish.collectionSettings')} description={t('polish.leaveBlankToUsePlatformDefaultsThe')} />
    <DetailList items={[{ label: t('polish.effectiveOrderLifetime'), value: t('polish.secondsValue', { v0: value.effective.order_ttl_seconds }) }, { label: t('polish.adjustmentLimitYuan'), value: centsToYuan(value.effective.float_limit_cents) }, { label: t('polish.adjustmentDirection'), value: value.effective.float_direction === 'up' ? t('polish.adjustUp') : t('polish.adjustDown') }]} />
    <form className="mt-5 flex flex-col gap-5" onSubmit={(event) => { event.preventDefault(); const next = collectionInput(ttl, amount, direction, value.bounds); setInvalid(!next); if (next) setPending(next) }}>
      <FieldGrid><Field label={t('polish.orderLifetimeSeconds')} htmlFor={`${id}-ttl`} hint={t('polish.ttlBounds', { v0: value.bounds.ttl_max_seconds })}><Input id={`${id}-ttl`} inputMode="numeric" value={ttl} onChange={(event) => setTTL(event.target.value)} /></Field>
        <Field label={t('polish.adjustmentLimitYuan')} htmlFor={`${id}-amount`} hint={t('polish.amountBounds', { v0: centsToYuan(value.bounds.float_min_cents), v1: centsToYuan(value.bounds.float_max_cents) })}><Input id={`${id}-amount`} inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} /></Field>
        <Field label={t('polish.adjustmentDirection')} htmlFor={`${id}-direction`}><Select id={`${id}-direction`} value={direction} onChange={(event) => setDirection(event.target.value)}><option value="">{t('polish.platformDefault')}</option><option value="up">{t('polish.adjustUp')}</option><option value="down">{t('polish.adjustDown')}</option></Select></Field>
      </FieldGrid>
      {invalid && <Notice tone="error">{t('polish.enterWholeSecondsAndAnAmountWith')}</Notice>}{action.error != null && !pending && !invalid && <ModuleError error={action.error} />}
      <FormActions className="border-t-[0.5px] border-border pt-5"><Button variant="primary" type="submit">{t('polish.saveCollectionSettings')}</Button></FormActions>
    </form>
    {pending && <Dialog title={t('polish.confirmCollectionSettings')} description={t('polish.theseSettingsApplyToNewOrdersReview')} busy={action.busy} onClose={() => setPending(null)} footer={<Button variant="primary" loading={action.busy} onClick={() => { void submit() }}>{action.busy ? t('polish.submitting') : t('polish.confirmSave')}</Button>}>{action.error != null && <ModuleError error={action.error} />}</Dialog>}
  </Card>
}
