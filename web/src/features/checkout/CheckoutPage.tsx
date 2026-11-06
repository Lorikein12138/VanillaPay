import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError } from '@/api/client'
import { LanguageSwitch } from '@/components/LanguageSwitch'
import { ThemeSwitch } from '@/components/ThemeSwitch'
import { Button } from '@/components/ui/button'
import { CopyText } from '@/components/ui/copy-button'
import { formatCountdown, useCountdown } from '@/components/ui/countdown'
import { Notice } from '@/components/ui/notice'
import { PaymentAmount, PaymentQR, PaymentResult, PaymentShell } from '@/components/ui/payment'
import { money } from '@/features/merchant/overview-data'
import { useCheckout } from './useCheckout'
import { returnToMerchant } from './return'

function PaidReturn({ url }: { url: string | null }) {
  const { t } = useTranslation()
  const [deadline] = useState(() => Date.now() + 3000)
  const seconds = useCountdown(url ? deadline : null)
  const returned = useRef(false)
  const go = () => { if (url && !returned.current) { returned.current = true; returnToMerchant(url) } }
  useEffect(() => {
    if (!url) return
    const timer = window.setTimeout(() => { if (!returned.current) { returned.current = true; returnToMerchant(url) } }, 3000)
    return () => window.clearTimeout(timer)
  }, [url])
  return <>{url ? <><p>{t('checkout.returnCountdown', { seconds })}</p><Button onClick={go}>{t('checkout.return')}</Button></> : <p>{t('checkout.close')}</p>}</>
}

export function CheckoutPage({ tradeNo }: { tradeNo: string }) {
  const { t, i18n } = useTranslation()
  const state = useCheckout(tradeNo)
  const data = state.data?.checkout
  const channelName = (channel: string) => t(`checkout.channels.${channel}`, { defaultValue: channel })
  const errorMessage = (error: unknown) => {
    const code = error instanceof APIError ? error.problem?.code : undefined
    if (code === 'AMOUNT_CAPACITY_EXHAUSTED') return t('checkout.capacity')
    if (code && ['CHANNEL_LOCKED', 'ORDER_NOT_PAYABLE', 'CHANNEL_NOT_ALLOWED', 'QR_UNAVAILABLE'].includes(code)) return t('checkout.changed')
    return t('checkout.retryError')
  }
  const terminal = data && ['paid', 'expired', 'refunded'].includes(data.status)
  const amount = data?.channel ? data.actual_cents : data?.original_cents
  return <PaymentShell merchant={data?.merchant?.username || 'VanillaPay'} controls={<><LanguageSwitch /><ThemeSwitch /></>} footer={t('checkout.footer')}>
    {state.missing ? <PaymentResult kind="unavailable" title={t('checkout.notFound')}><p>{t('checkout.notFoundHelp')}</p></PaymentResult>
      : !data ? state.isPending ? <p role="status">{t('checkout.loading')}</p> : <Notice tone="error">{t('checkout.retryError')} <Button variant="ghost" onClick={() => void state.refetch()}>{t('checkout.retry')}</Button></Notice>
        : terminal ? <><PaymentResult kind={data.status as 'paid' | 'expired' | 'refunded'} title={t(`checkout.status.${data.status}`, { defaultValue: data.status })}>
          {data.status === 'paid' ? <PaidReturn key={data.return_url_signed} url={data.return_url_signed} /> : <p>{t(`checkout.statusHelp.${data.status}`, { defaultValue: '' })}</p>}
        </PaymentResult><CopyText value={data.trade_no} label={t('checkout.order')} /></>
          : <>
            <PaymentAmount name={data.name} label={t(data.channel ? 'checkout.actual' : 'checkout.original')} amount={amount === null || amount === undefined ? '—' : money(amount, i18n.language)} original={data.channel && data.actual_cents !== null && data.actual_cents !== data.original_cents ? money(data.original_cents, i18n.language) : undefined} />
            {data.channel ? <>
              <p className="text-center text-label text-text-2">{channelName(data.channel)} · {t('checkout.locked')}</p>
              <Notice tone="warning">{data.actual_cents !== null && data.actual_cents !== data.original_cents ? t('checkout.exact', { amount: money(data.actual_cents, i18n.language) }) : t('checkout.exactSame')}</Notice>
            </> : <>
              <p className="text-center text-label text-text-2">{t('checkout.chooseHelp')}</p>
              <div className={(data.available_channels?.length ?? 0) > 1 ? 'grid grid-cols-1 gap-2 sm:grid-cols-2' : 'grid grid-cols-1 gap-2'} aria-label={t('checkout.choose')}>
                {(data.available_channels ?? []).map((channel) => <Button key={channel} variant="secondary" disabled={state.busy || data.reason !== 'no_channel'} onClick={() => void state.select(channel)}>{channelName(channel)}</Button>)}
              </div>
            </>}
            {(state.actionError || state.error) && <Notice tone="error">{errorMessage(state.actionError || state.error)}</Notice>}
            {data.can_pay && data.qr_image_url ? <><PaymentQR src={data.qr_image_url} alt={t('checkout.qr')} /><p className="text-center text-label text-text-2">{t('checkout.scan', { channel: channelName(data.channel ?? '') })}</p></>
              : <Notice tone={data.reason === 'no_channel' ? 'info' : 'warning'}>{t(`checkout.reasons.${data.reason ?? 'unavailable'}`, { defaultValue: t('checkout.reasons.unavailable') })}</Notice>}
            <p className="text-center text-caption text-text-3">{t('checkout.remaining')} <span className="font-mono tabular-nums">{formatCountdown(state.seconds)}</span></p>
            <CopyText value={data.trade_no} label={t('checkout.order')} />
          </>}
  </PaymentShell>
}
