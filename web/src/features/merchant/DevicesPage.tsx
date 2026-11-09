import { useTranslation } from 'react-i18next'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import QRCode from 'qrcode'
import { useCallback, useEffect, useState } from 'react'
import { api } from '@/api/client'
import { invalidateCollection, type ReceiptFilters, type Schema } from './collection-data'
import { ModuleError, useLocalAction } from './CollectionParts'
import { StatusDot } from '@/components/StatusDot'
import { Button } from '@/components/ui/button'
import { CopyField } from '@/components/ui/copy-button'
import { formatCountdown } from '@/components/ui/countdown'
import { DetailList } from '@/components/ui/detail-list'
import { Dialog } from '@/components/ui/dialog'
import { FormActions } from '@/components/ui/field'
import { Checkbox } from '@/components/ui/input'
import { Card, Page, PageHeader, SectionHeader } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'
import { OrderTime } from './OrderParts'
import { ReceiptsList } from './ReceiptsList'
import { MonitorDownload } from './MonitorDownload'

export function DevicesPage({ name, filters, onFilters }: { name: string; filters: ReceiptFilters; onFilters: (value: ReceiptFilters) => void }) {
  const { t } = useTranslation()
  const client = useQueryClient()
  const changed = useCallback(() => invalidateCollection(client, name), [client, name])
  const device = useQuery({ queryKey: ['merchant', name, 'device'], queryFn: api.device, refetchInterval: 15000 })
  const [dialog, setDialog] = useState<'bind' | 'unbind' | null>(null)
  return <Page>
    <PageHeader title={t('polish.monitoringDevice')} description={t('polish.bindAPaymentMonitorAndReviewDevice')} />
    {device.isError && <ModuleError error={device.error} retry={() => { void device.refetch() }} />}
    <Card aria-label={t('polish.deviceStatus')} aria-busy={device.isPending}>
      <SectionHeader title={device.data?.name ?? t('polish.paymentMonitor')} actions={<StatusDot tone={device.data?.online ? 'ok' : 'off'}>{device.data?.bound ? device.data.online ? t('polish.onlineBound') : t('polish.offlineBound') : t('polish.notBound')}</StatusDot>} />
      {device.data && <><DetailList columns={4} items={[
        { label: t('polish.lastHeartbeat'), value: <OrderTime value={device.data.last_seen_at} /> },
        { label: t('polish.appVersion'), value: device.data.app_version ?? '—' },
        { label: t('polish.heartbeatInterval'), value: t('polish.secondsValue', { v0: device.data.heartbeat_interval_seconds }) },
        { label: t('polish.offlineThreshold'), value: t('polish.secondsValue', { v0: device.data.offline_threshold_seconds }) },
      ]} /><FormActions className="mt-5"><Button variant="primary" onClick={() => setDialog('bind')}>{device.data.bound ? t('polish.replaceDevice') : t('polish.bindDevice')}</Button>{device.data.bound && <Button onClick={() => setDialog('unbind')}>{t('polish.unbindDevice')}</Button>}</FormActions></>}
    </Card>
    <MonitorDownload />
    <ReceiptsList name={name} filters={filters} onFilters={onFilters} />
    {dialog === 'bind' && device.data && <BindingDialog replace={device.data.bound} onClose={() => setDialog(null)} onChanged={changed} />}
    {dialog === 'unbind' && <UnbindDialog onClose={() => setDialog(null)} onChanged={changed} />}
  </Page>
}

export function BindingDialog({ replace, onClose, onChanged }: { replace: boolean; onClose: () => void; onChanged: () => Promise<void> }) {
  const { t } = useTranslation()
  const action = useLocalAction()
  const [binding, setBinding] = useState<Schema['DeviceBindingDTO'] | null>(null)
  const [confirmed, setConfirmed] = useState(!replace)
  const [status, setStatus] = useState('pending'), [remaining, setRemaining] = useState(0)
  const [qr, setQR] = useState(''), [pollError, setPollError] = useState<unknown>()
  async function generate() {
    await action.run(async () => {
      const result = await api.createBinding(replace)
      await onChanged()
      if (action.active.current) { setBinding(result); setStatus('pending'); setPollError(undefined) }
    })
  }
  useEffect(() => {
    if (!binding) return
    let active = true
    void QRCode.toDataURL(binding.qr_content, { width: 240, margin: 3, errorCorrectionLevel: 'M' }).then((url) => { if (active) setQR(url) }, () => { if (active) setPollError(new Error(t('polish.couldNotGenerateTheQrCodeUse'))) })
    return () => { active = false }
  }, [binding, t])
  useEffect(() => {
    if (!binding || status !== 'pending') return
    let active = true, timer: ReturnType<typeof setTimeout> | undefined
    const controller = new AbortController()
    const tick = () => {
      const seconds = Math.max(0, Math.ceil((Date.parse(binding.expires_at) - Date.now()) / 1000))
      setRemaining(seconds)
      if (!seconds) { setStatus('expired'); controller.abort() }
    }
    tick()
    const clock = setInterval(tick, 1000)
    async function poll() {
      try {
        const result = await api.binding(binding!.binding_id, controller.signal)
        if (!active || controller.signal.aborted) return
        setPollError(undefined)
        if (result.state === 'consumed') { setStatus('consumed'); await onChanged(); return }
        if (result.state === 'expired') { setStatus('expired'); return }
      } catch (error) { if (active && !controller.signal.aborted) setPollError(error) }
      if (active && !controller.signal.aborted) timer = setTimeout(() => { void poll() }, 2000)
    }
    void poll()
    return () => { active = false; controller.abort(); clearInterval(clock); clearTimeout(timer) }
  }, [binding, status, onChanged])
  return <Dialog title={t('polish.bindMonitoringDevice')} description={replace ? t('polish.thePreviousDeviceSCredentialsAreRevoked') : t('polish.scanTheQrCodeOrEnterThe')} busy={action.busy} onClose={onClose} footer={!binding ? <Button variant="primary" loading={action.busy} disabled={!confirmed} onClick={() => { void generate() }}>{action.busy ? t('polish.generating') : t('polish.generateBindingCode')}</Button> : status === 'expired' ? <Button variant="primary" loading={action.busy} onClick={() => { setQR(''); void generate() }}>{t('polish.generateAgain')}</Button> : undefined}>
    {!binding && <p className="text-label text-text-2">{t('monitorDownload.installFirst')}</p>}
    {!binding && replace && <Checkbox checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)}>{t('polish.iConfirmReplacingTheCurrentDevice')}</Checkbox>}
    {binding && <div className="flex flex-col gap-4">
      {status === 'pending' && <>{qr && <img className="mx-auto w-60 max-w-full rounded-control" src={qr} alt={t('polish.deviceBindingQrCode')} />}<p className="text-caption text-text-3">{t('polish.bindingCode')}</p><CopyField testId="binding-code" value={binding.code} /><div className="flex justify-center"><StatusDot tone="wait">{t('polish.bindingRemaining', { time: formatCountdown(remaining) })}</StatusDot></div><p className="text-center text-caption text-text-3">{t('polish.expires')}<OrderTime value={binding.expires_at} /></p></>}
      {status === 'consumed' && <Notice>{t('polish.deviceBound')}</Notice>}
      {status === 'expired' && <Notice tone="info">{t('polish.bindingCodeExpired')}</Notice>}
    </div>}
    {(action.error != null || pollError != null) && <ModuleError error={action.error ?? pollError} />}
  </Dialog>
}

function UnbindDialog({ onClose, onChanged }: { onClose: () => void; onChanged: () => Promise<void> }) {
  const { t } = useTranslation()
  const action = useLocalAction(), [confirmed, setConfirmed] = useState(false)
  return <Dialog title={t('polish.confirmDeviceUnbinding')} description={t('polish.unbindingImmediatelyRevokesDeviceCredentialsTheMonitor')} busy={action.busy} onClose={onClose} footer={<Button variant="primary" loading={action.busy} disabled={!confirmed} onClick={() => { void action.run(async () => { await api.unbindDevice(); await onChanged(); onClose() }) }}>{action.busy ? t('polish.submitting') : t('polish.confirmUnbind')}</Button>}>
    <Checkbox checked={confirmed} disabled={action.busy} onChange={(event) => setConfirmed(event.target.checked)}>{t('polish.iConfirmUnbindingTheCurrentDevice')}</Checkbox>
    {action.error != null && <ModuleError error={action.error} />}
  </Dialog>
}
