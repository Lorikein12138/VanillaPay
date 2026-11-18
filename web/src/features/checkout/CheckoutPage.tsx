import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, APIError } from '@/api/client'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { Button } from '@/components/ui/button'
import { formatCountdown, useCountdown } from '@/components/ui/countdown'
import { Notice } from '@/components/ui/notice'
import {
  ChannelBadge, ChannelOption, PaymentAmount, sortChannels, PaymentOrder, PaymentQR, PaymentResult, PaymentShell,
} from '@/components/ui/payment'
import { money } from '@/features/merchant/overview-data'
import { Turnstile } from '@/features/auth/Turnstile'
import { useCheckout } from './useCheckout'
import { returnToMerchant } from './return'

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
      : <p className="text-label text-text-2">{t('checkout.verificationHelp')}</p>}
    <Turnstile key={attempt} siteKey={siteKey} action="checkout" onToken={setToken} onError={widgetFailed} />
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
  return <>{url ? <><p>{t('checkout.returnCountdown', { seconds })}</p><Button variant="primary" onClick={go}>{t('checkout.return')}</Button></> : <p>{t('checkout.close')}</p>}</>
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
  const merchant = data?.merchant?.username || 'VanillaPay'
  const status = data?.status
  const terminal = status === 'paid' || status === 'expired' || status === 'refunded'
  const gated = data?.verification?.required === true
  const locked = Boolean(data?.channel && !gated)
  const qrSource = data?.can_pay ? data.qr_image_url : null
  const amount = locked ? data?.actual_cents : data?.original_cents
  const original = locked && data?.actual_cents != null && data.actual_cents !== data.original_cents
    ? <>{t('merchant.orders.originalAmount')} <del>{money(data.original_cents, i18n.language)}</del></>
    : undefined
  const receipt = !blocked && !state.missing && data && (
    <section className="receipt-paper min-w-0 px-4 pt-2.5 pb-2 md:px-8 md:pt-6 md:pb-5">
      <div className="flex flex-wrap items-baseline justify-between gap-x-2 border-b border-receipt-rule pb-1.5 md:pb-4">
        <span className="text-body font-bold tracking-tight md:text-lg" translate="no">VanillaPay</span>
        <span className="text-caption text-text-2">{t('checkout.receipt')}</span>
      </div>
      {terminal
        ? <p className="mt-2 mb-1 text-label font-semibold md:mt-4 md:mb-2 md:text-lg">{data.name}</p>
        : <h1 className="mt-2 mb-1 font-serif text-body font-semibold md:mt-4 md:mb-2 md:text-heading">{data.name}</h1>}
      <PaymentAmount label={t(locked ? 'checkout.actual' : 'checkout.original')}
        amount={amount == null ? '—' : money(amount, i18n.language)} original={original} />
      <PaymentOrder value={data.trade_no} label={t('checkout.orderLabel')} copyLabel={t('checkout.order')} />
    </section>
  )

  const actionHeading = 'font-serif text-[clamp(1.375rem,2.6vw,2.25rem)] leading-tight'
  const error = state.actionError || state.error
  const reasonNotice = data?.reason !== 'no_channel' && (
    <Notice tone="warning" className="gap-2 px-2.5 py-2">
      {t(`checkout.reasons.${data?.reason ?? 'unavailable'}`, { defaultValue: t('checkout.reasons.unavailable') })}
    </Notice>
  )
  const timeRemaining = <p className="text-caption text-text-2">{t('checkout.remaining')} <span className="font-mono tabular-nums">{formatCountdown(state.seconds)}</span></p>

  return (
    <PaymentShell merchant={merchant} controls={<><LanguageSwitch /><ThemeSwitch /></>} footer={t('checkout.footer')} receipt={receipt}>
      {blocked ? (
        <PaymentResult kind="blocked" title={t('checkout.ipBlocked')}><p>{t('checkout.ipBlockedHelp')}</p></PaymentResult>
      ) : state.missing ? (
        <PaymentResult kind="unavailable" title={t('checkout.notFound')}><p>{t('checkout.notFoundHelp')}</p></PaymentResult>
      ) : !data ? (
        state.isPending
          ? <h1 role="status" className={actionHeading}>{t('checkout.loading')}</h1>
          : <PaymentResult kind="unavailable" title={t('error.title')}>
              <p>{t('checkout.retryError')}</p>
              <Button variant="primary" onClick={() => void state.refetch()}>{t('checkout.retry')}</Button>
            </PaymentResult>
      ) : terminal ? (
        <PaymentResult kind={status} title={t(`checkout.status.${status}`)}>
          {status === 'paid' ? (
            <>
              {data.actual_cents !== null && (
                <p>
                  {t('checkout.paidAmount', { amount: money(data.actual_cents, i18n.language) })}
                  {data.channel && <> · {channelName(data.channel)}</>}
                </p>
              )}
              <PaidReturn key={tradeNo} url={data.return_url_signed} hasReturned={hasReturned} onReturn={returnOnce} />
            </>
          ) : <p>{t(`checkout.statusHelp.${status}`)}</p>}
        </PaymentResult>
      ) : gated && data.verification ? (
        <>
          <h2 className={actionHeading}>{t('checkout.verificationTitle')}</h2>
          <CheckoutVerification key={tradeNo} tradeNo={tradeNo} siteKey={data.verification.site_key}
            onVerified={() => void state.refetch()} onBlocked={() => void state.refetch()} />
          {timeRemaining}
        </>
      ) : (
        <>
          <h2 ref={data.channel && qrSource ? scanTitle : undefined} tabIndex={data.channel && qrSource ? -1 : undefined} className={actionHeading}>
            {t(qrSource ? 'checkout.qr' : data.channel ? 'error.title' : 'checkout.choose')}
          </h2>
          {data.channel ? (
            <>
              <div className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center justify-start gap-2.5 border-t border-border pt-2.5 md:gap-4 md:pt-4">
                {qrSource && <PaymentQR src={qrSource} alt={t('checkout.qr')} />}
                <div className={qrSource ? 'min-w-0 space-y-1' : 'col-span-2 min-w-0 space-y-1'}>
                  <ChannelBadge channel={data.channel} name={channelName(data.channel)} label={t('checkout.locked')} />
                  {qrSource && <p className="text-label text-text-2">{t('checkout.scan', { channel: channelName(data.channel) })}</p>}
                  {timeRemaining}
                </div>
              </div>
              <Notice tone="warning" className="gap-2 px-2.5 py-2">
                {data.actual_cents !== null && data.actual_cents !== data.original_cents
                  ? t('checkout.exact', { amount: money(data.actual_cents, i18n.language) })
                  : t('checkout.exactSame')}
              </Notice>
            </>
          ) : (
            <>
              {!error && data.reason === 'no_channel' && <p className="text-label text-text-2">{t('checkout.chooseHelp')}</p>}
              <div role="group" aria-label={t('checkout.choose')} aria-busy={state.busy} className="grid gap-2">
                {sortChannels(data.available_channels ?? []).map((channel) => (
                  <ChannelOption key={channel} channel={channel} disabled={state.busy || data.reason !== 'no_channel'}
                    onClick={(event) => void chooseChannel(channel, event.currentTarget)}><span translate="no">{channelName(channel)}</span></ChannelOption>
                ))}
              </div>
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
