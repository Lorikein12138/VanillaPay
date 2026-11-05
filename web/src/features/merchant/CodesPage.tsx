import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { APIError, api } from '@/api/client'
import { FormError } from '@/components/FormError'
import { collectionInput, centsToYuan, invalidateCollection, uploadError, type Schema } from './collection-data'
import { ModuleDialog, ModuleError, ModuleHeading, modulePanel, useLocalAction } from './CollectionParts'
import { orderChannels } from './orders-data'
import { OrderChannel, OrderTime, orderButton, orderInput, orderPrimary } from './OrderParts'

export function CodesPage({ name }: { name: string }) {
  const [notice, setNotice] = useState('')
  const collection = useQuery({ queryKey: ['merchant', name, 'collection'], queryFn: api.collection })
  const codes = useQuery({ queryKey: ['merchant', name, 'codes'], queryFn: api.qrSlots })
  return <div className="mx-auto flex max-w-[1112px] flex-col gap-8">
    <ModuleHeading title="收款码">管理各渠道收款码与订单收款设置。</ModuleHeading>
    {notice && <p role="status" className="text-label">{notice}</p>}
    {collection.isError && <ModuleError error={collection.error} retry={() => { void collection.refetch() }} />}
    {codes.isError && <ModuleError error={codes.error} retry={() => { void codes.refetch() }} />}
    <section aria-label="收款码槽位" aria-busy={codes.isPending} className="grid gap-5 lg:grid-cols-3">
      {(codes.data || collection.data) && orderChannels.map((channel) => <QRSlot key={channel} name={name} channel={channel} value={(codes.data?.codes ?? collection.data?.codes)?.find((code) => code.channel === channel)} />)}
      {codes.isPending && <p className="text-label text-text-2">正在加载收款码…</p>}
    </section>
    {collection.data && <CollectionForm key={JSON.stringify(collection.data.settings)} name={name} value={collection.data.settings} onNotice={setNotice} />}
  </div>
}

function QRSlot({ name, channel, value }: { name: string; channel: string; value?: Schema['SettingsQRDTO'] }) {
  const client = useQueryClient(), action = useLocalAction(), id = useId()
  const [file, setFile] = useState<File | null>(null)
  const [invalid, setInvalid] = useState('')
  const [confirm, setConfirm] = useState<'upload' | 'toggle' | null>(null)
  const [success, setSuccess] = useState('')
  async function submit() {
    await action.run(async () => {
      if (confirm === 'upload' && file) {
        // Decode locally as well as checking MIME/size; the server validates QR content.
        const bitmap = await createImageBitmap(file)
        const valid = bitmap.width <= 4096 && bitmap.height <= 4096
        bitmap.close()
        if (!valid) throw new Error('图片宽、高均不能超过 4096 像素。')
        await api.uploadQR(channel, file)
      } else if (confirm === 'toggle') await api.setQREnabled(channel, !value?.enabled)
      await invalidateCollection(client, name)
      setConfirm(null); setFile(null); setSuccess('收款码已更新。')
    })
  }
  return <article className={modulePanel} aria-label={`${channel}收款码`}>
    <div className="flex items-center justify-between gap-2"><h2 className="font-serif text-xl"><OrderChannel channel={channel} /></h2><span className="text-caption text-text-2">{value?.asset_id ? value.enabled ? '已启用' : '已停用' : '未上传'}</span></div>
    <div className="my-5 flex min-h-44 items-center justify-center rounded-control bg-muted p-4">{value?.image_url ? <img className="max-h-44 max-w-full rounded-control bg-white p-2" src={value.image_url} alt={`${channel}收款码预览`} /> : <p className="text-label text-text-3">上传后在此预览</p>}</div>
    <p className="mb-4 text-caption text-text-3">更新时间：<OrderTime value={value?.updated_at ?? null} /></p>
    <label htmlFor={id} className="text-label">{value?.asset_id ? '替换图片' : '上传图片'}</label><input id={id} type="file" className="my-2 w-full min-w-0 text-caption" accept="image/png,image/jpeg" disabled={action.busy} onChange={(event) => { const next = event.target.files?.[0]; event.target.value = ''; setSuccess(''); if (next) { const error = uploadError(next); setInvalid(error ?? ''); if (!error) { setFile(next); setConfirm('upload') } } }} />
    <p className="text-caption leading-relaxed text-text-3">PNG / JPEG · 最大 5 MiB · 最大 4096 × 4096</p>
    {value?.asset_id && <button className={`${orderButton} mt-4`} onClick={() => setConfirm('toggle')}>{value.enabled ? '停用' : '启用'}</button>}
    {invalid && <FormError>{invalid}</FormError>}{success && <p role="status" className="mt-3 text-label">{success}</p>}
    {confirm && <ModuleDialog title={confirm === 'upload' ? '确认上传收款码' : '确认更改启用状态'} description={confirm === 'upload' ? '新图片会替换当前渠道的收款码，并自动启用。请确认图片属于此渠道。' : '更改后将影响新订单的收款渠道可用性。'} busy={action.busy} onClose={() => { setConfirm(null); setFile(null) }}>
      {file && <p className="mb-4 break-all text-label">{file.name}</p>}{action.error != null && <ModuleError error={action.error} />}
      <button className={`${orderPrimary} mt-4`} disabled={action.busy} onClick={() => { void submit() }}>{action.busy ? '提交中…' : '确认保存'}</button>
    </ModuleDialog>}
  </article>
}

function CollectionForm({ name, value, onNotice }: { name: string; value: Schema['SettingsCollectionSettings']; onNotice: (value: string) => void }) {
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
      catch (error) { if (error instanceof APIError && error.status === 409) { setPending(null); onNotice('设置版本已变化，已重新加载，请核对后重试。'); await invalidateCollection(client, name) }; throw error }
      setPending(null); onNotice('收款设置已保存。'); await invalidateCollection(client, name)
    })
  }
  return <section className={modulePanel}><h2 className="font-serif text-2xl">收款设置</h2>
    <p className="my-3 text-label text-text-2">当前生效：有效期 {value.effective.order_ttl_seconds} 秒，浮动上限 {centsToYuan(value.effective.float_limit_cents)} 元，{value.effective.float_direction === 'up' ? '向上浮动' : '向下浮动'}。版本 {value.version}。</p>
    <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); const next = collectionInput(ttl, amount, direction, value.bounds); setInvalid(!next); if (next) setPending(next) }}>
      <p className="text-caption text-text-3">留空表示跟随平台默认值；最终生效值以保存后响应为准。</p>
      <div className="grid gap-4 sm:grid-cols-3">
        <label className="space-y-2 text-label" htmlFor={`${id}-ttl`}><span>订单有效期（秒）</span><input id={`${id}-ttl`} aria-label="订单有效期（秒）" className={orderInput} inputMode="numeric" value={ttl} onChange={(event) => setTTL(event.target.value)} /><span className="block text-caption text-text-3">60 – {value.bounds.ttl_max_seconds} 秒</span></label>
        <label className="space-y-2 text-label" htmlFor={`${id}-amount`}><span>浮动上限（元）</span><input id={`${id}-amount`} aria-label="浮动上限（元）" className={orderInput} inputMode="decimal" value={amount} onChange={(event) => setAmount(event.target.value)} /><span className="block text-caption text-text-3">{centsToYuan(value.bounds.float_min_cents)} – {centsToYuan(value.bounds.float_max_cents)} 元</span></label>
        <label className="space-y-2 text-label" htmlFor={`${id}-direction`}><span>浮动方向</span><select id={`${id}-direction`} aria-label="浮动方向" className={orderInput} value={direction} onChange={(event) => setDirection(event.target.value)}><option value="">跟随平台</option><option value="up">向上浮动</option><option value="down">向下浮动</option></select></label>
      </div>
      {invalid && <FormError>请按平台范围填写整数秒和最多两位小数的金额。</FormError>}{action.error != null && !pending && <ModuleError error={action.error} />}
      <button className={orderPrimary} type="submit">保存收款设置</button>
    </form>
    {pending && <ModuleDialog title="确认保存收款设置" description="保存后将应用于新订单，请核对范围和浮动方向。" busy={action.busy} onClose={() => setPending(null)}>{action.error != null && <ModuleError error={action.error} />}<button className={orderPrimary} disabled={action.busy} onClick={() => { void submit() }}>{action.busy ? '提交中…' : '确认保存'}</button></ModuleDialog>}
  </section>
}
