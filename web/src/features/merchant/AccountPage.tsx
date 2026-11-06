import { useQueryClient } from '@tanstack/react-query'
import { useNavigate } from '@tanstack/react-router'
import QRCode from 'qrcode'
import { useEffect, useState } from 'react'
import { api, APIError } from '@/api/client'
import { FormError } from '@/components/FormError'
import { AccountError, AccountField, EmailCodeButton, useEmailCode, useSecondsUntil, validPassword, validUsername, type AccountSchema } from '@/features/auth/account-shared'
import { useMerchantAccount } from './AccountProvider'
import { CopyValue, ModuleDialog, ModuleHeading, modulePanel, useLocalAction } from './CollectionParts'
import { OrderTime, orderButton, orderPrimary } from './OrderParts'

export function AccountPage() {
  const account = useMerchantAccount()
  const [dialog, setDialog] = useState<'username' | 'email' | 'password' | 'setup' | 'disable' | null>(null)
  if (!account) return null
  const { user } = account
  return <div className="mx-auto flex max-w-[1112px] flex-col gap-8"><ModuleHeading title="账户安全">管理商户资料、登录验证与个人偏好。</ModuleHeading>
    <section className={modulePanel}><h2 className="font-serif text-2xl">商户资料</h2><dl className="my-6 grid gap-5 text-label sm:grid-cols-2"><div><dt className="mb-2 text-text-3">用户名</dt><dd className="break-all">{user.username}</dd></div><div><dt className="mb-2 text-text-3">邮箱</dt><dd className="break-all">{user.email ?? '—'}</dd></div><div><dt className="mb-2 text-text-3">PID</dt><dd className="break-all font-mono">{user.pid}</dd></div><div><dt className="mb-2 text-text-3">注册时间</dt><dd><OrderTime value={user.created_at} /></dd></div></dl><div className="flex flex-wrap gap-3">{(['username', 'email', 'password'] as const).map((mode) => <button key={mode} className={orderButton} onClick={() => setDialog(mode)}>修改{mode === 'username' ? '用户名' : mode === 'email' ? '邮箱' : '密码'}</button>)}</div></section>
    <section className={modulePanel}><h2 className="font-serif text-2xl">两步验证</h2><p className="my-4 text-label text-text-2">{user.totp_enabled ? '已开启。登录和敏感操作需要验证器动态验证码。' : '尚未开启。使用验证器应用保护登录和敏感操作。'}</p><button className={orderPrimary} onClick={() => setDialog(user.totp_enabled ? 'disable' : 'setup')}>{user.totp_enabled ? '关闭两步验证' : '开启两步验证'}</button></section>
    <section className={modulePanel}><h2 className="font-serif text-2xl">个人偏好</h2><p className="my-3 text-label text-text-2">登录后以服务器保存的偏好为准，顶栏的切换也会同步保存。</p><div className="grid gap-5 sm:grid-cols-2"><label className="space-y-2 text-label"><span className="block">语言</span><select aria-label="账户语言" className={orderButton} disabled={account.busy} value={account.preferences.locale} onChange={(event) => { void account.save({ ...account.preferences, locale: event.target.value === 'en' ? 'en' : 'zh-CN' }) }}><option value="zh-CN">中文</option><option value="en">English</option></select></label><label className="space-y-2 text-label"><span className="block">主题</span><select aria-label="账户主题" className={orderButton} disabled={account.busy} value={account.preferences.theme} onChange={(event) => { const theme = event.target.value; if (theme === 'system' || theme === 'light' || theme === 'dark') void account.save({ ...account.preferences, theme }) }}><option value="system">跟随系统</option><option value="light">浅色</option><option value="dark">深色</option></select></label></div></section>
    {dialog && (dialog === 'setup' || dialog === 'disable' ? <FactorDialog mode={dialog} onClose={() => setDialog(null)} onChanged={account.refresh} /> : <IdentityDialog mode={dialog} user={user} onClose={() => setDialog(null)} onChanged={account.refresh} />)}
  </div>
}

export function IdentityDialog({ mode, user, onClose, onChanged }: { mode: 'username' | 'email' | 'password'; user: AccountSchema['AuthUserDTO']; onClose: () => void; onChanged: () => Promise<void> }) {
  const action = useLocalAction(), mail = useEmailCode(), newMail = useEmailCode(), client = useQueryClient(), navigate = useNavigate()
  const [value, setValue] = useState(''), [confirmation, setConfirmation] = useState(''), [code, setCode] = useState(''), [newCode, setNewCode] = useState(''), [totp, setTOTP] = useState(''), [invalid, setInvalid] = useState('')
  const title = `修改${mode === 'username' ? '用户名' : mode === 'email' ? '邮箱' : '密码'}`
  const busy = action.busy || mail.busy || newMail.busy
  const valid = mode === 'username' ? validUsername(value) : mode === 'email' ? /^[^\s@]+@[^\s@]+$/.test(value.trim()) && value.trim().toLowerCase() !== user.email?.toLowerCase() : validPassword(value, confirmation)
  async function sendOld() {
    if (!user.email || (mode === 'username' && !valid)) return
    await mail.send({ email: user.email, purpose: mode === 'username' ? 'change_username' : mode === 'email' ? 'change_email_old' : 'change_password', ...(mode === 'username' ? { target_username: value.trim() } : { target_email: user.email }) })
  }
  async function submit() {
    if (!valid || !code.trim() || (mode === 'email' && !newCode.trim()) || (user.totp_enabled && !/^\d{6}$/.test(totp))) { setInvalid('请检查资料、邮箱验证码和所需的六位动态验证码。密码须为两次一致的 12–128 个字符。'); return }
    setInvalid('')
    await action.run(async () => {
      const proof: AccountSchema['AuthSecurityProof'] = { email_code: code.trim(), ...(user.totp_enabled ? { totp_code: totp } : {}) }
      if (mode === 'password') {
        await api.changePassword({ password: value, password_confirmation: confirmation, security_proof: proof })
        api.forgetSession(); client.clear()
        if (action.active.current) { setValue(''); setConfirmation(''); setCode(''); setTOTP(''); await navigate({ to: '/login', search: { notice: 'password_changed' }, replace: true }) }
      } else {
        if (mode === 'username') await api.changeUsername({ username: value.trim(), security_proof: proof })
        else await api.changeEmail({ email: value.trim(), old_email_code: code.trim(), new_email_code: newCode.trim(), ...(user.totp_enabled ? { totp_code: totp } : {}) })
        if (action.active.current) { setCode(''); setNewCode(''); setTOTP(''); await onChanged(); if (action.active.current) onClose() }
      }
    })
  }
  return <ModuleDialog title={title} description={mode === 'password' ? '保存后所有会话立即失效，需要重新登录。' : '需要当前邮箱验证码；开启两步验证时还需要动态验证码。'} busy={busy} onClose={onClose}><form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void submit() }}>
    <AccountField label={mode === 'username' ? '新用户名' : mode === 'email' ? '新邮箱' : '新密码'} type={mode === 'password' ? 'password' : mode === 'email' ? 'email' : 'text'} autoComplete={mode === 'password' ? 'new-password' : mode === 'email' ? 'email' : 'username'} required disabled={busy} value={value} onChange={(event) => { setValue(event.target.value); if (mode === 'username') setCode(''); if (mode === 'email') setNewCode('') }} />
    {mode === 'password' && <AccountField label="确认新密码" type="password" autoComplete="new-password" required disabled={busy} value={confirmation} onChange={(event) => setConfirmation(event.target.value)} />}
    <p className="break-all text-caption text-text-2">当前邮箱：{user.email ?? '无法读取，请重新登录'}</p>
    <EmailCodeButton mail={mail} label={mode === 'email' ? '获取旧邮箱验证码' : undefined} disabled={busy || !user.email || (mode === 'username' && !valid)} onSend={() => { void sendOld() }} />
    <AccountField label={mode === 'email' ? '旧邮箱验证码' : '邮箱验证码'} autoComplete="one-time-code" inputMode="numeric" required disabled={busy} value={code} onChange={(event) => setCode(event.target.value)} />
    {mode === 'email' && <><EmailCodeButton mail={newMail} label="获取新邮箱验证码" disabled={busy || !valid} onSend={() => { void newMail.send({ email: value.trim(), target_email: value.trim(), purpose: 'change_email_new' }) }} /><AccountField label="新邮箱验证码" autoComplete="one-time-code" inputMode="numeric" required disabled={busy} value={newCode} onChange={(event) => setNewCode(event.target.value)} /></>}
    {user.totp_enabled && <AccountField label="动态验证码" autoComplete="one-time-code" inputMode="numeric" maxLength={6} required disabled={busy} value={totp} onChange={(event) => setTOTP(event.target.value)} />}
    {invalid && <FormError>{invalid}</FormError>}{action.error != null && <AccountError error={action.error} />}<button className={orderPrimary} disabled={busy || !valid || !code.trim() || (mode === 'email' && !newCode.trim()) || (user.totp_enabled && !/^\d{6}$/.test(totp))}>{busy ? '保存中…' : '确认保存'}</button>
  </form></ModuleDialog>
}

export function FactorDialog({ mode, onClose, onChanged }: { mode: 'setup' | 'disable'; onClose: () => void; onChanged: () => Promise<void> }) {
  const action = useLocalAction()
  const [password, setPassword] = useState(''), [code, setCode] = useState('')
  const [setup, setSetup] = useState<AccountSchema['AuthTOTPSetup'] | null>(null), [recovery, setRecovery] = useState<string[] | null>(null), [expired, setExpired] = useState(false)
  const remaining = useSecondsUntil(setup ? Date.parse(setup.expires_at) : 0)
  useEffect(() => {
    if (!setup) return
    const timer = setTimeout(() => { setSetup(null); setCode(''); setExpired(true) }, Math.max(0, Date.parse(setup.expires_at) - Date.now()))
    return () => clearTimeout(timer)
  }, [setup])
  function close() { setSetup(null); setRecovery(null); setPassword(''); setCode(''); onClose() }
  async function submit() {
    if ((mode === 'disable' || !setup) && !password) return
    if ((mode === 'disable' || setup) && !/^\d{6}$/.test(code)) return
    await action.run(async () => {
      if (mode === 'disable') {
        await api.disableTOTP({ password, totp_code: code })
        if (action.active.current) { setPassword(''); setCode(''); await onChanged(); if (action.active.current) close() }
      } else if (!setup) {
        const result = await api.setupTOTP({ password })
        if (action.active.current) { setPassword(''); const valid = Date.parse(result.expires_at) > Date.now(); setSetup(valid ? result : null); setExpired(!valid) }
      } else {
        try {
          const result = await api.confirmTOTP({ setup_id: setup.setup_id, totp_code: code })
          if (action.active.current) { setSetup(null); setCode(''); setRecovery(result.recovery_codes ?? []); await onChanged() }
        } catch (error) {
          if (action.active.current && error instanceof APIError && error.problem?.code === 'CHALLENGE_EXPIRED') { setSetup(null); setCode(''); setExpired(true) }
          throw error
        }
      }
    })
  }
  function download() {
    if (!recovery) return
    const url = URL.createObjectURL(new Blob([recovery.join('\n') + '\n'], { type: 'text/plain;charset=utf-8' }))
    const link = document.createElement('a'); link.href = url; link.download = 'vanillapay-recovery-codes.txt'; link.click()
    setTimeout(() => URL.revokeObjectURL(url), 0)
  }
  return <ModuleDialog title={recovery ? '保存恢复码' : mode === 'setup' ? '开启两步验证' : '关闭两步验证'} description={recovery ? '恢复码仅展示本次，每个只能使用一次。请保存在安全位置；关闭弹窗后清除。' : mode === 'setup' ? '先验证当前密码，再用验证器扫描二维码并输入六位动态验证码。' : '需要当前密码和一个新的动态验证码，不需要邮箱验证码。'} busy={action.busy} onClose={close}>
    {recovery ? <div className="space-y-4"><pre data-testid="recovery-codes" className="whitespace-pre-wrap break-all rounded-control bg-muted p-4 font-mono text-label">{recovery.join('\n')}</pre><div className="flex flex-wrap gap-2"><CopyValue value={recovery.join('\n')} label="复制恢复码" /><button className={orderButton} onClick={download}>下载恢复码 TXT</button></div></div> : <form className="space-y-4" onSubmit={(event) => { event.preventDefault(); void submit() }}>
      {expired && <p role="status" className="text-label text-danger">设置已过期，请重新输入密码生成。</p>}
      {setup ? <><TOTPSecret setup={setup} /><p role="status" className="text-center text-caption text-text-3">设置剩余 {remaining} 秒</p></> : <AccountField label="当前密码" type="password" autoComplete="current-password" required value={password} disabled={action.busy} onChange={(event) => setPassword(event.target.value)} />}
      {(setup || mode === 'disable') && <AccountField label="动态验证码" autoComplete="one-time-code" inputMode="numeric" maxLength={6} required value={code} disabled={action.busy} onChange={(event) => setCode(event.target.value)} />}
      {action.error != null && <AccountError error={action.error} />}<button className={orderPrimary} disabled={action.busy || (!setup && !password) || ((setup || mode === 'disable') && !/^\d{6}$/.test(code))}>{action.busy ? '处理中…' : mode === 'disable' ? '确认关闭两步验证' : setup ? '确认开启' : '验证密码并生成'}</button>
    </form>}
  </ModuleDialog>
}
function TOTPSecret({ setup }: { setup: AccountSchema['AuthTOTPSetup'] }) {
  const [qr, setQR] = useState(''), [failed, setFailed] = useState(false)
  useEffect(() => {
    let active = true
    void QRCode.toDataURL(setup.otpauth_url, { width: 240, margin: 3 }).then((result) => { if (active) setQR(result) }, () => { if (active) setFailed(true) })
    return () => { active = false }
  }, [setup])
  return <div className="space-y-3 text-center">{qr && <img data-testid="totp-qr" className="mx-auto w-60 max-w-full rounded-control" src={qr} alt="验证器设置二维码" />}{failed && <p className="text-caption text-danger">二维码生成失败，请手动输入密钥。</p>}<p className="text-caption text-text-2">手动输入密钥</p><pre data-testid="totp-secret" className="whitespace-pre-wrap break-all rounded-control bg-muted p-3 font-mono text-label">{setup.secret}</pre><CopyValue value={setup.secret} label="复制验证器密钥" /></div>
}
