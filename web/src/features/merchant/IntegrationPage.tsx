import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useId, useRef, useState } from 'react'
import { api } from '@/api/client'
import { useEmailCode } from '@/features/auth/account-shared'
import { FormError } from '@/components/FormError'
import { invalidateCollection, validWhitelist, type Schema } from './collection-data'
import { CopyValue, ModuleDialog, ModuleError, ModuleHeading, modulePanel, useLocalAction } from './CollectionParts'
import { orderButton, orderInput, orderPrimary } from './OrderParts'

export function IntegrationPage({ user }: { user: Schema['AuthUserDTO'] }) {
  const name = user.username, client = useQueryClient()
  const integration = useQuery({ queryKey: ['merchant', name, 'integration'], queryFn: async () => {
    const data = await api.integration()
    // This endpoint returns a secret. Discard it before entering the query cache.
    return { ...data, key: '' }
  }, gcTime: 0 })
  const [dialog, setDialog] = useState<'reset' | 'rsa' | null>(null)
  const [reveal, setReveal] = useState(false), [currentKey, setCurrentKey] = useState('')
  const secretAction = useLocalAction(), revealIntent = useRef(false)
  useEffect(() => () => { revealIntent.current = false }, [])
  function hide() { revealIntent.current = false; setReveal(false); setCurrentKey('') }
  const data = integration.data
  return <div className="mx-auto flex max-w-[1112px] flex-col gap-8">
    <ModuleHeading title="接入配置">管理支付接口、密钥与访问控制。</ModuleHeading>
    {integration.isPending && <p>正在加载接入配置…</p>}{integration.isError && <ModuleError error={integration.error} retry={() => { void integration.refetch() }} />}
    {data && <>
      <section className={modulePanel}><h2 className="font-serif text-2xl">商户凭据</h2><dl className="my-5 grid gap-4 text-label sm:grid-cols-2"><div><dt className="mb-1 text-text-3">PID</dt><dd className="break-all font-mono">{data.pid}</dd></div><div><dt className="mb-1 text-text-3">密钥版本</dt><dd>{data.key_version}</dd></div><div className="sm:col-span-2"><dt className="mb-1 text-text-3">接口地址</dt><dd className="break-all font-mono">{data.base_url}</dd></div><div className="sm:col-span-2"><dt className="mb-2 text-text-3">商户 Key</dt><dd className="space-y-3"><p className="break-all font-mono">{reveal ? currentKey : '••••••••••••••••••••••••••••••••'}</p><div className="flex flex-wrap gap-2"><button className={orderButton} disabled={secretAction.busy} onClick={() => {
        if (reveal) { hide(); return }
        revealIntent.current = true
        void secretAction.run(async () => { const result = await api.integration(); if (secretAction.active.current && revealIntent.current) { setCurrentKey(result.key); setReveal(true) } })
      }}>{secretAction.busy ? '读取中…' : reveal ? '隐藏 Key' : '显示 Key'}</button>{reveal && <CopyValue value={currentKey} label="复制 Key" />}<button className={orderButton} onClick={() => { hide(); setDialog('reset') }}>重置 Key</button></div></dd></div></dl>
        {secretAction.error != null && <ModuleError error={secretAction.error} />}
      </section>
      <section className={modulePanel}><h2 className="font-serif text-2xl">接口说明</h2><p className="my-3 text-label text-text-2">v1 使用商户 Key 进行 MD5 签名；v2 使用 RSA 签名，请配置商户公钥，并使用平台公钥验签。请在服务端保存密钥。</p><div className="grid gap-6 md:grid-cols-2">{(['v1', 'v2'] as const).map((version) => <div key={version}><h3 className="mb-3 font-mono">{version}</h3><dl className="space-y-3 text-label">{Object.entries(data[version]).map(([operation, url]) => <div key={operation}><dt className="text-text-3">{operation}</dt><dd className="mt-1 break-all font-mono text-caption">{url}</dd></div>)}</dl></div>)}</div></section>
      <section className={modulePanel}><div className="flex flex-wrap items-center justify-between gap-3"><h2 className="font-serif text-2xl">RSA 公钥</h2><button className={orderButton} onClick={() => setDialog('rsa')}>保存商户公钥</button></div><h3 className="mb-2 mt-5 text-label">商户公钥</h3><pre className="whitespace-pre-wrap break-all rounded-control bg-muted p-4 text-caption">{data.merchant_public_key ?? '尚未配置'}</pre><h3 className="mb-2 mt-5 text-label">平台公钥</h3>{data.platform_public_keys.length ? data.platform_public_keys.map((item) => <div key={item.id} className="mt-3"><p className="mb-2 break-all font-mono text-caption">{item.id}</p><pre className="whitespace-pre-wrap break-all rounded-control bg-muted p-4 text-caption">{item.pem}</pre><div className="mt-2"><CopyValue value={item.pem} label="复制平台公钥" /></div></div>) : <p className="text-label text-text-3">暂无平台公钥</p>}</section>
      <WhitelistForm key={JSON.stringify(data.ip_whitelist)} name={name} value={data.ip_whitelist} />
    </>}
    {dialog && <SecurityDialog mode={dialog} user={user} onClose={() => setDialog(null)} onChanged={() => invalidateCollection(client, name)} />}
  </div>
}

export function SecurityDialog({ mode, user, onClose, onChanged }: { mode: 'reset' | 'rsa'; user: Schema['AuthUserDTO']; onClose: () => void; onChanged: () => Promise<void> }) {
  const action = useLocalAction(), mail = useEmailCode(), id = useId()
  const [emailCode, setEmailCode] = useState(''), [totpCode, setTOTPCode] = useState(''), [pem, setPEM] = useState('')
  const [newKey, setNewKey] = useState<string | null>(null), [invalid, setInvalid] = useState(''), [saved, setSaved] = useState(false)
  const { remaining, sent } = mail
  function close() { setNewKey(null); setEmailCode(''); setTOTPCode(''); onClose() }
  async function send() {
    if (!user.email) return
    await mail.send({ email: user.email, purpose: 'reset_key' })
  }
  async function submit() {
    if (!emailCode.trim() || (user.totp_enabled && !/^\d{6}$/.test(totpCode))) { setInvalid('请填写邮箱验证码和所需的六位动态验证码。'); return }
    if (mode === 'rsa' && !/^-----BEGIN (RSA )?PUBLIC KEY-----\s+[A-Za-z0-9+/=\s]+\s+-----END (RSA )?PUBLIC KEY-----$/.test(pem.trim())) { setInvalid('请输入 PEM 格式的 RSA 公钥，不要粘贴私钥。'); return }
    setInvalid('')
    await action.run(async () => {
      const proof: Schema['AuthSecurityProof'] = { email_code: emailCode.trim(), ...(user.totp_enabled ? { totp_code: totpCode } : {}) }
      if (mode === 'reset') {
        const result = await api.resetKey(proof)
        if (action.active.current) setNewKey(result.key)
      } else { await api.saveRSA({ public_key_pem: pem.trim(), security_proof: proof }); if (action.active.current) setSaved(true) }
      setEmailCode(''); setTOTPCode(''); await onChanged()
    })
  }
  return <ModuleDialog title={newKey !== null ? '新 Key 已生成' : mode === 'reset' ? '重置商户 Key' : '保存 RSA 公钥'} description={newKey !== null ? '请立即复制并妥善保管。关闭此弹窗将清除本次展示内容。' : mode === 'reset' ? '重置后旧 Key 立即失效。需要当前商户邮箱验证码；已开启两步验证时还需要动态验证码。' : '使用 reset_key 用途的邮箱验证码验证身份。后端将检查公钥类型与强度。'} busy={action.busy || mail.busy} onClose={close}>
    {newKey !== null ? <div className="space-y-5"><pre data-testid="one-time-key" className="whitespace-pre-wrap break-all rounded-control bg-muted p-4 font-mono text-label">{newKey}</pre><CopyValue value={newKey} label="复制新 Key" /></div> : saved ? <p role="status">RSA 公钥已保存。</p> : <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void submit() }}>
      {mode === 'rsa' && <label className="block space-y-2 text-label" htmlFor={`${id}-pem`}><span>RSA 公钥 PEM</span><textarea id={`${id}-pem`} className={`${orderInput} font-mono text-caption`} rows={7} maxLength={60000} value={pem} disabled={action.busy} onChange={(event) => setPEM(event.target.value)} /></label>}
      <p className="break-all text-label text-text-2">验证邮箱：{user.email ?? '无法读取当前邮箱，请刷新登录状态'}</p>
      <button type="button" className={orderButton} disabled={!user.email || mail.busy || action.busy || remaining > 0} onClick={() => { void send() }}>{mail.busy ? '发送中…' : remaining > 0 ? `${remaining} 秒后重发` : '获取邮箱验证码'}</button>
      {sent && <p role="status" className="text-caption text-text-2">验证码已发送，请查收邮箱。</p>}{mail.error != null && <ModuleError error={mail.error} />}
      <label className="block space-y-2 text-label" htmlFor={`${id}-email`}><span>邮箱验证码</span><input id={`${id}-email`} className={orderInput} autoComplete="one-time-code" inputMode="numeric" required value={emailCode} disabled={action.busy} onChange={(event) => setEmailCode(event.target.value)} /></label>
      {user.totp_enabled && <label className="block space-y-2 text-label" htmlFor={`${id}-totp`}><span>动态验证码</span><input id={`${id}-totp`} className={orderInput} autoComplete="off" inputMode="numeric" maxLength={6} required value={totpCode} disabled={action.busy} onChange={(event) => setTOTPCode(event.target.value)} /></label>}
      {invalid && <FormError>{invalid}</FormError>}{action.error != null && <ModuleError error={action.error} />}
      <button type="submit" className={orderPrimary} disabled={action.busy || mail.busy || !emailCode.trim() || (user.totp_enabled && !/^\d{6}$/.test(totpCode))}>{action.busy ? '提交中…' : mode === 'reset' ? '确认重置 Key' : '确认保存公钥'}</button>
    </form>}
  </ModuleDialog>
}

function WhitelistForm({ name, value }: { name: string; value: Schema['MerchantIPWhitelist'] }) {
  const client = useQueryClient(), action = useLocalAction(), id = useId()
  const [enabled, setEnabled] = useState(value.enabled), [text, setText] = useState((value.entries ?? []).join('\n')), [invalid, setInvalid] = useState(false)
  const [pending, setPending] = useState<Schema['MerchantIPWhitelist'] | null>(null)
  return <section className={modulePanel}><h2 className="font-serif text-2xl">IP 白名单</h2><p className="my-3 text-label leading-relaxed text-text-2">每行一个 IPv4、IPv6 地址或 CIDR，最多 100 条。开启时列表不能为空，后端会拒绝保存空列表。关闭白名单后不限制请求 IP。</p><form className="space-y-4" onSubmit={(event) => { event.preventDefault(); const entries = text.split(/\r?\n/).map((item) => item.trim()).filter(Boolean); if (!validWhitelist(entries, enabled)) { setInvalid(true); return }; setInvalid(false); setPending({ enabled, entries }) }}>
    <label className="flex items-center gap-2 text-label"><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />启用 IP 白名单</label>
    <label htmlFor={id} className="block space-y-2 text-label"><span>允许的 IP 或 CIDR</span><textarea id={id} className={`${orderInput} font-mono`} rows={5} value={text} onChange={(event) => setText(event.target.value)} /></label>{invalid && <FormError>请填写最多 100 个有效 IP / CIDR；开启时至少填写一条。</FormError>}<button type="submit" className={orderPrimary}>保存白名单</button>
  </form>{pending && <ModuleDialog title="确认保存 IP 白名单" description="请确认业务服务器出口 IP 已包含在列表中。后端会规范化并校验条目。" busy={action.busy} onClose={() => setPending(null)}>{action.error != null && <ModuleError error={action.error} />}<button className={orderPrimary} disabled={action.busy} onClick={() => { void action.run(async () => { await api.saveWhitelist(pending); setPending(null); await invalidateCollection(client, name) }) }}>{action.busy ? '提交中…' : '确认保存白名单'}</button></ModuleDialog>}</section>
}
