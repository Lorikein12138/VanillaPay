import type { ReactNode } from 'react'
import { Check, ChevronRight, CircleAlert, Clock, RotateCcw, ShieldX } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button, type ButtonProps } from './button'
import { channelPaths } from './channel-paths'
import { CopyButton } from './copy-button'

export const paymentHeadingClass = 'text-[20px]/[26px] font-bold'

export function PaymentShell({ controls, receiptLabel, summary, footer, order, children }: {
  controls: ReactNode; receiptLabel: ReactNode; summary?: ReactNode; footer: ReactNode; order?: ReactNode; children: ReactNode
}) {
  const { t } = useTranslation()
  return <div className="checkout-stage flex min-h-dvh flex-col">
    <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:z-10 focus:rounded-control focus:bg-surface focus:p-3">{t('common.skipToContent')}</a>
    <div className="grid flex-1 place-items-center px-3 pt-3.5 pb-5">
      <div className="checkout-paper w-full min-w-0 max-w-[492px] [overflow-wrap:anywhere]">
        <header className="flex items-center justify-between gap-2">
          <div className="min-w-0">
            <p className="font-bold tracking-tight" translate="no">VanillaPay</p>
            <p className="checkout-small">{receiptLabel}</p>
          </div>
          <div className="flex shrink-0">{controls}</div>
        </header>
        <main id="main" tabIndex={-1} className="flow-root">
          {summary}
          <div className="checkout-section flex min-w-0 flex-col gap-2">
            {children}
          </div>
          <footer className="checkout-section checkout-small">
            {order}
            <p className="mt-1">{footer}</p>
          </footer>
        </main>
      </div>
    </div>
  </div>
}

/** Display order shared by every channel list; unknown channels keep their relative order at the end. */
const channelOrder = Object.keys(channelPaths)
export function sortChannels<T extends string>(channels: readonly T[]): T[] {
  const rank = (channel: string) => { const index = channelOrder.indexOf(channel); return index < 0 ? channelOrder.length : index }
  return [...channels].sort((a, b) => rank(a) - rank(b))
}

// Shared brand tokens, with brighter dark-theme variants for legibility.
const channelColors: Record<keyof typeof channelPaths, string> = {
  alipay: 'bg-[var(--vp-alipay-tile)] text-[var(--vp-alipay)]',
  wxpay: 'bg-[var(--vp-wxpay-tile)] text-[var(--vp-wxpay)]',
}
/**
 * `brand` uses a coloured tile, `glyph` uses the brand colour without a tile,
 * and `plain` uses a muted glyph for dense lists and tables.
 */
export function ChannelIcon({ channel, size = 'sm', tone = 'brand', className }: {
  channel: string; size?: 'sm' | 'md'; tone?: 'brand' | 'glyph' | 'plain'; className?: string
}) {
  const brand = Object.hasOwn(channelPaths, channel) ? channel as keyof typeof channelPaths : null
  if (tone !== 'brand') {
    return brand
      ? <svg aria-hidden="true" className={cn('shrink-0', tone === 'plain' ? 'size-3.5 text-text-3' : 'size-4 text-(--checkout-channel)', className)}
          data-channel={tone === 'glyph' ? brand : undefined} viewBox="0 0 24 24" fill="currentColor"><path d={channelPaths[brand]} /></svg>
      : null
  }
  return (
    <span aria-hidden="true" className={cn(
      'inline-grid shrink-0 place-items-center',
      size === 'md' ? 'size-6 rounded-[6px]' : 'size-4 rounded-[4px]',
      brand ? channelColors[brand] : 'bg-muted text-text-3',
      className,
    )}>
      {brand && (
        <svg className="size-[62.5%]!" viewBox="0 0 24 24" fill="currentColor">
          <path d={channelPaths[brand]} />
        </svg>
      )}
    </span>
  )
}

export function ChannelLabel({ channel, children }: { channel: string; children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap align-middle">
      <ChannelIcon channel={channel} tone="plain" />
      {children}
    </span>
  )
}

export function ChannelList({ channels, name }: { channels: string[]; name: (channel: string) => string }) {
  return (
    <span className="inline-flex flex-wrap items-center gap-x-4 gap-y-1">
      {sortChannels(channels).map((channel) => <ChannelLabel key={channel} channel={channel}>{name(channel)}</ChannelLabel>)}
    </span>
  )
}

export function ChannelOption({ channel, selected = false, children, className, ...props }: ButtonProps & { channel: string; selected?: boolean }) {
  return (
    <Button {...props} variant="ghost" aria-pressed={selected} className={cn(
      'min-h-12 w-full min-w-0 justify-start rounded-none border-b border-(--checkout-rule) px-0 py-1.5 text-start text-[15px]/[22px] whitespace-normal text-inherit hover:bg-(--checkout-hover) hover:text-inherit focus-visible:outline-offset-[-2px]',
      selected && 'bg-accent-soft hover:bg-accent-soft', className,
    )}>
      <ChannelIcon channel={channel} tone="glyph" />
      <span className="min-w-0 flex-1">{children}</span>
      <ChevronRight className="me-1 text-(--checkout-muted)" aria-hidden="true" />
    </Button>
  )
}

export function ChannelBadge({ channel, name, label }: { channel: string; name: string; label?: string }) {
  return <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
    <span className="inline-flex min-w-0 items-center gap-2 font-semibold" translate="no">
      <ChannelIcon channel={channel} tone="glyph" />
      {name}
    </span>
    {label && <span className="checkout-small">{label}</span>}
  </div>
}

export function PaymentOrder({ label, value, copyLabel }: { label: string; value: string; copyLabel: string }) {
  return <dl className="flex min-w-0 flex-wrap items-center gap-x-2">
    <dt>{label}</dt>
    <dd className="inline-flex min-w-0 max-w-full items-center gap-0.5" translate="no">
      <span className="min-w-0 tabular-nums">{value}</span>
      <CopyButton value={value} label={copyLabel} size="sm" className="text-(--checkout-accent)" />
    </dd>
  </dl>
}

export function PaymentAmount({ label, amount, original }: { label: ReactNode; amount: ReactNode; original?: ReactNode }) {
  return <div className="checkout-section">
    <p className="checkout-small">{label}</p>
    <p className="font-serif text-[44px]/[48px] font-semibold tracking-[-0.045em] lining-nums tabular-nums min-[560px]:text-[56px]/[60px]">{amount}</p>
    {original && <p className="checkout-small">{original}</p>}
  </div>
}

export function PaymentQR({ src, alt }: { src: string; alt: string }) {
  return <div className="w-[min(8rem,35vw)] max-w-full shrink-0 bg-white p-2">
    <img width={128} height={128} className="aspect-square w-full object-contain" src={src} alt={alt} />
  </div>
}

export function PaymentResult({ kind, title, children, heading: Heading = 'h1' }: {
  kind: 'paid' | 'expired' | 'refunded' | 'unavailable' | 'blocked'; title: ReactNode; children: ReactNode; heading?: 'h1' | 'h2'
}) {
  const Icon = { paid: Check, expired: Clock, refunded: RotateCcw, unavailable: CircleAlert, blocked: ShieldX }[kind]
  return <div className="grid gap-2">
    <Heading className={cn(paymentHeadingClass, 'flex items-center gap-2')}>
      <Icon className="size-[18px] shrink-0 text-(--checkout-accent)" aria-hidden="true" />
      {title}
    </Heading>
    <div className="flex min-w-0 flex-col items-start gap-1">{children}</div>
  </div>
}
