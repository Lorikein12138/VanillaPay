import { useQuery, useQueryClient } from '@tanstack/react-query'
import QRCode from 'qrcode'
import { useCallback, useEffect, useState } from 'react'
import { api } from '@/api/client'
import { invalidateCollection, type ReceiptFilters, type Schema } from './collection-data'
import { ModuleDialog, ModuleError, ModuleHeading, modulePanel, useLocalAction } from './CollectionParts'
import { OrderTime, orderButton, orderPrimary } from './OrderParts'
import { ReceiptsList } from './ReceiptsList'

export function DevicesPage({ name, filters, onFilters }: { name: string; filters: ReceiptFilters; onFilters: (value: ReceiptFilters) => void }) {
  const client = useQueryClient()
  const changed = useCallback(() => invalidateCollection(client, name), [client, name])
  const device = useQuery({ queryKey: ['merchant', name, 'device'], queryFn: api.device, refetchInterval: 15000 })
  const [dialog, setDialog] = useState<'bind' | 'unbind' | null>(null)
  return <div className="mx-auto flex max-w-[1112px] flex-col gap-8">
    <ModuleHeading title="监控设备">绑定收款监控端，查看设备状态与收款记录。</ModuleHeading>
    {device.isError && <ModuleError error={device.error} retry={() => { void device.refetch() }} />}
    <section className={modulePanel} aria-label="设备状态" aria-busy={device.isPending}>
      <div className="flex flex-wrap items-center justify-between gap-4"><h2 className="font-serif text-2xl">{device.data?.name ?? '收款监控端'}</h2><span className="flex items-center gap-2 text-label"><span className={`size-2 rounded-full ${device.data?.online ? 'bg-dot-ok' : 'bg-dot-off'}`} />{device.data?.bound ? device.data.online ? '在线 · 已绑定' : '离线 · 已绑定' : '未绑定'}</span></div>
      {device.data && <><dl className="my-6 grid gap-5 text-label sm:grid-cols-2 lg:grid-cols-4">
        <div><dt className="mb-2 text-text-3">最后心跳</dt><dd><OrderTime value={device.data.last_seen_at} /></dd></div>
        <div><dt className="mb-2 text-text-3">应用版本</dt><dd>{device.data.app_version ?? '—'}</dd></div>
        <div><dt className="mb-2 text-text-3">心跳间隔</dt><dd>{device.data.heartbeat_interval_seconds} 秒</dd></div>
        <div><dt className="mb-2 text-text-3">离线阈值</dt><dd>{device.data.offline_threshold_seconds} 秒</dd></div>
      </dl><div className="flex flex-wrap gap-3"><button className={orderPrimary} onClick={() => setDialog('bind')}>{device.data.bound ? '更换设备' : '绑定设备'}</button>{device.data.bound && <button className={orderButton} onClick={() => setDialog('unbind')}>解绑设备</button>}</div></>}
    </section>
    <ReceiptsList name={name} filters={filters} onFilters={onFilters} />
    {dialog === 'bind' && device.data && <BindingDialog replace={device.data.bound} onClose={() => setDialog(null)} onChanged={changed} />}
    {dialog === 'unbind' && <UnbindDialog onClose={() => setDialog(null)} onChanged={changed} />}
  </div>
}

export function BindingDialog({ replace, onClose, onChanged }: { replace: boolean; onClose: () => void; onChanged: () => Promise<void> }) {
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
    void QRCode.toDataURL(binding.qr_content, { width: 240, margin: 3, errorCorrectionLevel: 'M' }).then((url) => { if (active) setQR(url) }, () => { if (active) setPollError(new Error('无法生成二维码，可使用下方绑定码。')) })
    return () => { active = false }
  }, [binding])
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
  return <ModuleDialog title="绑定监控设备" description={replace ? '新设备绑定成功后，原设备凭据会立即失效。' : '在监控端扫描二维码或输入绑定码。绑定码仅在倒计时内有效。'} busy={action.busy} onClose={onClose}>
    {!binding && <div className="space-y-4">{replace && <label className="flex items-start gap-2 text-label"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} />我确认替换当前设备</label>}<button className={orderPrimary} disabled={!confirmed || action.busy} onClick={() => { void generate() }}>{action.busy ? '生成中…' : '生成绑定码'}</button></div>}
    {binding && <div className="space-y-4 text-center">
      {status === 'pending' && <>{qr && <img className="mx-auto w-60 max-w-full rounded-control" src={qr} alt="设备绑定二维码" />}<p className="text-caption text-text-3">绑定码</p><p data-testid="binding-code" className="break-all font-mono text-2xl tracking-wider">{binding.code}</p><p role="status" className="text-label">等待绑定 · 剩余 {remaining} 秒</p><p className="text-caption text-text-3">到期时间：<OrderTime value={binding.expires_at} /></p></>}
      {status === 'consumed' && <p role="status" className="py-8 text-xl">设备已绑定</p>}
      {status === 'expired' && <><p role="status">绑定码已过期</p><button className={orderPrimary} disabled={action.busy} onClick={() => { setQR(''); void generate() }}>重新生成</button></>}
    </div>}
    {action.error != null && <ModuleError error={action.error} />}{pollError != null && <ModuleError error={pollError} />}
  </ModuleDialog>
}

function UnbindDialog({ onClose, onChanged }: { onClose: () => void; onChanged: () => Promise<void> }) {
  const action = useLocalAction(), [confirmed, setConfirmed] = useState(false)
  return <ModuleDialog title="确认解绑设备" description="解绑后设备凭据立即失效，监控端将无法再上报收款记录。" busy={action.busy} onClose={onClose}>
    <label className="mb-5 flex items-center gap-2 text-label"><input type="checkbox" checked={confirmed} disabled={action.busy} onChange={(event) => setConfirmed(event.target.checked)} />我确认解绑当前设备</label>
    {action.error != null && <ModuleError error={action.error} />}<button className={orderPrimary} disabled={!confirmed || action.busy} onClick={() => { void action.run(async () => { await api.unbindDevice(); await onChanged(); onClose() }) }}>{action.busy ? '提交中…' : '确认解绑'}</button>
  </ModuleDialog>
}
