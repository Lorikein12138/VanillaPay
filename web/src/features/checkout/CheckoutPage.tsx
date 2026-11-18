import { useCallback, useEffect, useRef, useState } from 'react'
import { Globe, Monitor, Moon, Sun } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import { Button } from '@/components/ui/button'
import { formatCountdown, useCountdown } from '@/components/ui/countdown'
import { Notice } from '@/components/ui/notice'
import {
  ChannelBadge, ChannelOption, PaymentAmount, paymentHeadingClass, sortChannels, PaymentOrder, PaymentQR, PaymentResult, PaymentShell,
} from '@/components/ui/payment'
import { money } from '@/features/merchant/overview-data'
import { Turnstile } from '@/features/auth/Turnstile'
import { changeLanguage } from '@/lib/i18n'
import { setTheme, useTheme } from '@/lib/theme'
import { useCheckout } from './useCheckout'
import { returnToMerchant } from './return'
import './checkout.css'

const themeControls = {
  system: { icon: Monitor, next: 'light' },
  light: { icon: Sun, next: 'dark' },
  dark: { icon: Moon, next: 'system' },
} as const

function CheckoutControls() {
  const { t, i18n } = useTranslation()
  const theme = useTheme()
  const { icon: ThemeIcon, next } = themeControls[theme]
  const language = i18n.resolvedLanguage === 'en' ? 'zh-CN' : 'en'
  const languageLabel = t('checkout.switchLanguage')
  const themeLabel = t('checkout.cycleTheme', { current: t(`theme.${theme}`), next: t(`theme.${next}`) })
  const controlClass = 'text-(--checkout-muted) hover:bg-transparent hover:text-inherit focus-visible:outline-offset-[-2px] [&_svg]:size-[18px]'

  return <>
    <Button variant="ghost" size="icon" className={controlClass} aria-label={languageLabel} title={languageLabel}
      onClick={() => void changeLanguage(i18n, language)}>
      <Globe aria-hidden="true" strokeWidth={1.6} />
    </Button>
    <Button variant="ghost" size="icon" className={controlClass} aria-label={themeLabel} title={themeLabel}
      data-theme-preference={theme} onClick={() => setTheme(next)}>
      <ThemeIcon aria-hidden="true" strokeWidth={1.6} />
    </Button>
  </>
}

function CheckoutVerification({ tradeNo, siteKey, onVerified, onBlocked }: {
  tradeNo: string
  siteKey: string
  onVerified: () => void
  onBlocked: () => void
}) {
  const { t } = useTranslation()
  const [token, setToken] = useState('')
  const [attempt, setAttempt] = useState(0)
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState(false)
  const widgetFailed = useCallback(() => setFailed(true), [])
  async function verify() {
    if (!token || busy) return
    setBusy(true)
    setFailed(false)
    try {
      await api.verifyCheckout(tradeNo, { turnstile_token: token })
      onVerified()
    } catch (error) {
      if (error instanceof APIError && error.problem?.code === 'IP_BLOCKED') onBlocked()
      setFailed(true)
      setToken('')
      setAttempt((value) => value + 1)
    } finally {
      setBusy(false)
    }
  }
  return <div className="grid min-w-0 gap-2.5">
    {failed
      ? <Notice tone="error" className="gap-2 px-2.5 py-2 [&>div]:leading-snug">{t('checkout.verificationFailed')}</Notice>
      : <p>{t('checkout.verificationHelp')}</p>}
    <div className="min-w-0 overflow-x-auto">
      <div className="min-w-[150px]">
        <Turnstile key={attempt} siteKey={siteKey} action="checkout" onToken={setToken} onError={widgetFailed} />
      </div>
    </div>
    <div className="flex flex-wrap items-center gap-2">
      <Button variant="primary" loading={busy} disabled={!token} onClick={() => void verify()}>{t('checkout.verify')}</Button>
      {failed && <Button variant="ghost" onClick={() => {
        setFailed(false)
        setAttempt((value) => value + 1)
      }}>{t('checkout.retry')}</Button>}
    </div>
  </div>
}

function PaidReturn({ url, hasReturned, onReturn }: { url: string | null; hasReturned: () => boolean; onReturn: (url: string) => void }) {
  const { t } = useTranslation()
  const [deadline] = useState(() => Date.now() + 3000)
  const seconds = useCountdown(url ? deadline : null)
  const timer = useRef<number | undefined>(undefined)
  const go = useCallback(() => {
    if (!url) return
    window.clearTimeout(timer.current)
    onReturn(url)
  }, [onReturn, url])
  useEffect(() => {
    if (!url || hasReturned()) return
    // A refreshed signature updates the destination, never the original deadline.
    timer.current = window.setTimeout(go, Math.max(0, deadline - Date.now()))
    return () => window.clearTimeout(timer.current)
  }, [deadline, go, hasReturned, url])
  return <>{url ? <><p>{t('checkout.returnCountdown', { seconds })}</p><Button variant="link" className="text-[15px] font-semibold underline" onClick={go}>{t('checkout.return')}</Button></> : <p>{t('checkout.close')}</p>}</>
}

export function CheckoutPage({ tradeNo }: { tradeNo: string }) {
  const { t, i18n } = useTranslation()
  const state = useCheckout(tradeNo)
  const data = state.data?.checkout
  const scanTitle = useRef<HTMLHeadingElement>(null)
  const [scanFocus, setScanFocus] = useState<{ snapshot: NonNullable<typeof state.data>; trigger: HTMLButtonElement }>()
  useEffect(() => {
    // Wait for the GET triggered by this selection to reach the DOM.
    if (!scanFocus || scanFocus.snapshot !== state.data) return
    const active = document.activeElement
    if (active === document.body || active === scanFocus.trigger) scanTitle.current?.focus()
  }, [scanFocus, state.data])

  async function chooseChannel(channel: string, trigger: HTMLButtonElement) {
    const snapshot = await state.select(channel)
    const checkout = snapshot?.checkout
    if (snapshot && checkout?.status === 'pending' && checkout.channel === channel && checkout.can_pay &&
      checkout.qr_image_url && !checkout.verification?.required) setScanFocus({ snapshot, trigger })
  }

  const blocked = [state.error, state.actionError].some((error) => error instanceof APIError && error.problem?.code === 'IP_BLOCKED')
  // Survives signed URL changes and paid-result remounts for this page lifecycle.
  const returnedOrders = useRef(new Set<string>())
  const hasReturned = useCallback(() => returnedOrders.current.has(tradeNo), [tradeNo])
  const returnOnce = useCallback((url: string) => {
    if (returnedOrders.current.has(tradeNo)) return
    returnedOrders.current.add(tradeNo)
    returnToMerchant(url)
  }, [tradeNo])
  const channelName = (channel: string) => t(`checkout.channels.${channel}`, { defaultValue: channel })
  const errorMessage = (error: unknown) => {
    const code = error instanceof APIError ? error.problem?.code : undefined
    if (code === 'AMOUNT_CAPACITY_EXHAUSTED') return t('checkout.capacity')
    if (code && ['CHANNEL_LOCKED', 'ORDER_NOT_PAYABLE', 'CHANNEL_NOT_ALLOWED', 'QR_UNAVAILABLE'].includes(code)) return t('checkout.changed')
    return t('checkout.retryError')
  }
  const status = data?.status
  const terminal = status === 'paid' || status === 'expired' || status === 'refunded'
  const gated = data?.verification?.required === true
  const verifying = !terminal && gated && !blocked && !state.missing
  const locked = Boolean(data?.channel && !gated)
  const qrSource = data?.can_pay ? data.qr_image_url : null
  const paidAmount = status === 'paid' && data?.actual_cents != null
  const expired = status === 'expired'
  const showActual = !expired && (locked || paidAmount)
  const amount = showActual ? data?.actual_cents : data?.original_cents
  const original = showActual && locked && data?.actual_cents != null && data.actual_cents !== data.original_cents
    ? <>{t('merchant.orders.originalAmount')} <del>{money(data.original_cents, i18n.language)}</del></>
    : undefined
  const visibleOrder = !blocked && !state.missing && !verifying ? data : undefined
  const merchant = visibleOrder?.merchant?.username
  const summary = visibleOrder && (
    <>
      <dl className="checkout-section grid gap-0.5">
        {merchant && <div className="flex min-w-0 items-baseline gap-1">
          <dt className="shrink-0 text-(--checkout-muted)">{t('checkout.merchantLabel')}</dt>
          <dd className="min-w-0 font-semibold" translate="no">{merchant}</dd>
        </div>}
        <div className="flex min-w-0 items-baseline gap-1">
          <dt className="shrink-0 text-(--checkout-muted)">{t('checkout.productLabel')}</dt>
          <dd className="min-w-0"><h1 className="font-semibold">{visibleOrder.name}</h1></dd>
        </div>
      </dl>
      <PaymentAmount label={t(expired ? 'checkout.orderAmount' : paidAmount ? 'checkout.paidLabel' : locked ? 'checkout.actual' : 'checkout.original')}
        amount={amount == null ? '—' : money(amount, i18n.language)} original={original} />
    </>
  )

  const error = state.actionError || state.error
  const reasonNotice = data?.reason !== 'no_channel' && (
    <Notice tone="warning" className="gap-2 px-2.5 py-2">
      {t(`checkout.reasons.${data?.reason ?? 'unavailable'}`, { defaultValue: t('checkout.reasons.unavailable') })}
    </Notice>
  )
  const timeRemaining = <p className="checkout-small">{t('checkout.remaining')} <span className="tabular-nums">{formatCountdown(state.seconds)}</span></p>

  return (
    <PaymentShell controls={<CheckoutControls />} receiptLabel={t(verifying ? 'checkout.title' : 'checkout.receipt')} summary={summary}
      footer={t('checkout.footer')} order={visibleOrder && <PaymentOrder value={visibleOrder.trade_no} label={t('checkout.orderLabel')} copyLabel={t('checkout.order')} />}>
      <span aria-live="polite" className="sr-only">{visibleOrder && terminal ? t(`checkout.status.${status}`) : ''}</span>
      {blocked ? (
        <PaymentResult kind="blocked" title={t('checkout.ipBlocked')}><p>{t('checkout.ipBlockedHelp')}</p></PaymentResult>
      ) : state.missing ? (
        <PaymentResult kind="unavailable" title={t('checkout.notFound')}><p>{t('checkout.notFoundHelp')}</p></PaymentResult>
      ) : !data ? (
        state.isPending
          ? <div role="status"><h1 className={paymentHeadingClass}>{t('checkout.loading')}</h1></div>
          : <PaymentResult kind="unavailable" title={t('error.title')}>
              <p>{t('checkout.retryError')}</p>
              <Button variant="primary" onClick={() => void state.refetch()}>{t('checkout.retry')}</Button>
            </PaymentResult>
      ) : terminal ? (
        <PaymentResult kind={status} title={t(`checkout.status.${status}`)} heading="h2">
          {status === 'paid' ? (
            <>
              {data.channel && <ChannelBadge channel={data.channel} name={channelName(data.channel)} />}
              <PaidReturn key={tradeNo} url={data.return_url_signed} hasReturned={hasReturned} onReturn={returnOnce} />
            </>
          ) : <p>{t(`checkout.statusHelp.${status}`)}</p>}
        </PaymentResult>
      ) : verifying && data.verification ? (
        <>
          <h1 className={paymentHeadingClass}>{t('checkout.verificationTitle')}</h1>
          <CheckoutVerification key={tradeNo} tradeNo={tradeNo} siteKey={data.verification.site_key}
            onVerified={() => void state.refetch()} onBlocked={() => void state.refetch()} />
          {timeRemaining}
        </>
      ) : (
        <>
          <h2 ref={data.channel && qrSource ? scanTitle : undefined} tabIndex={data.channel && qrSource ? -1 : undefined} className={paymentHeadingClass}>
            {t(qrSource ? 'checkout.qr' : data.channel ? 'error.title' : 'checkout.choose')}
          </h2>
          {data.channel ? (
            <>
              <div className="flex min-w-0 flex-wrap items-center gap-3">
                {qrSource && <PaymentQR src={qrSource} alt={t('checkout.qr')} />}
                <div className="grid min-w-0 flex-[1_1_120px] gap-0.5">
                  <ChannelBadge channel={data.channel} name={channelName(data.channel)} label={t('checkout.locked')} />
                  {qrSource && <p>{t('checkout.scan', { channel: channelName(data.channel) })}</p>}
                  {timeRemaining}
                </div>
              </div>
              <Notice tone="warning" className="gap-2 px-2.5 py-2">
                {data.actual_cents !== null && data.actual_cents !== data.original_cents
                  ? t('checkout.exact')
                  : t('checkout.exactSame')}
              </Notice>
            </>
          ) : (
            <>
              {!error && data.reason === 'no_channel' && <p>{t('checkout.chooseHelp')}</p>}
              <div role="group" aria-label={t('checkout.choose')} aria-busy={state.busy} className="grid not-empty:border-t not-empty:border-(--checkout-rule)">
                {sortChannels(data.available_channels ?? []).map((channel) => (
                  <ChannelOption key={channel} channel={channel} disabled={state.busy || data.reason !== 'no_channel'}
                    onClick={(event) => void chooseChannel(channel, event.currentTarget)}><span translate="no">{channelName(channel)}</span></ChannelOption>
                ))}
              </div>
              {state.busy && <p role="status" className="checkout-small">{t('checkout.selecting')}</p>}
              {timeRemaining}
            </>
          )}
          {error && <Notice tone="error" className="gap-2 px-2.5 py-2">{errorMessage(error)}</Notice>}
          {!qrSource && reasonNotice}
        </>
      )}
    </PaymentShell>
  )
}
