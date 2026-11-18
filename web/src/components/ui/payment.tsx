import type { ReactNode } from 'react'
import { Check, ChevronRight, CircleAlert, Clock, RotateCcw, ShieldX, Store } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button, type ButtonProps } from './button'
import { channelPaths } from './channel-paths'
import { CopyText } from './copy-button'

export function PaymentShell({ merchant, controls, footer, receipt, children }: {
  merchant: ReactNode; controls: ReactNode; footer: ReactNode; receipt?: ReactNode; children: ReactNode
}) {
  const { t } = useTranslation()
  return <div className="flex min-h-dvh flex-col bg-stage">
    <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:z-10 focus:rounded-control focus:bg-surface focus:p-3">{t('common.skipToContent')}</a>
    <header className="border-b border-border bg-bg px-3 py-1 sm:px-6">
      <div className="mx-auto flex max-w-[1120px] items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Store className="size-5 shrink-0 text-accent-text" strokeWidth={1.7} aria-hidden="true" />
          <span className="min-w-0 truncate font-serif text-lg" translate="no">{merchant}</span>
        </div>
        <div className="flex shrink-0 items-center gap-0.5">{controls}</div>
      </div>
    </header>
    <main id="main" tabIndex={-1} className="grid flex-1 place-items-center px-3 py-2 sm:px-6 sm:py-6">
      <div className={cn('grid w-full min-w-0 gap-x-4 gap-y-2.5 [overflow-wrap:anywhere]',
        receipt ? 'max-w-[1120px] gap-y-3 md:grid-cols-[minmax(0,0.85fr)_minmax(0,1.15fr)] md:gap-x-[clamp(1.5rem,4vw,4rem)]' : 'max-w-lg')}>
        {receipt}
        <section className="flex min-w-0 flex-col justify-center gap-2 rounded-dialog border border-border bg-surface px-4 py-2.5 shadow-dialog md:gap-4 md:px-9 md:py-7">
          {children}
        </section>
        <footer className="text-end text-caption text-text-3 md:col-span-full">{footer}</footer>
      </div>
    </main>
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
 * `brand` is a coloured tile for places where the channel is the thing being
 * chosen or identified (checkout, collection-code cards). `plain` is a muted
 * glyph for dense lists and tables, where colour would only add noise.
 */
export function ChannelIcon({ channel, size = 'sm', tone = 'brand', className }: {
  channel: string; size?: 'sm' | 'md'; tone?: 'brand' | 'plain'; className?: string
}) {
  const brand = Object.hasOwn(channelPaths, channel) ? channel as keyof typeof channelPaths : null
  if (tone === 'plain') {
    return brand
      ? <svg aria-hidden="true" className={cn('size-3.5 shrink-0 text-text-3', className)} viewBox="0 0 24 24" fill="currentColor"><path d={channelPaths[brand]} /></svg>
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
    <Button {...props} aria-pressed={selected} className={cn(
      'min-h-11 w-full min-w-0 justify-start gap-2.5 whitespace-normal border-border bg-field px-2 py-1.5 text-start md:min-h-14 md:gap-3',
      selected && 'border-accent bg-accent-soft hover:bg-accent-soft', className,
    )}>
      <ChannelIcon channel={channel} size="md" className="size-8 rounded-control [&_svg]:size-5 md:size-10" />
      <span className="min-w-0 flex-1">{children}</span>
      <ChevronRight className="text-accent-text" aria-hidden="true" />
    </Button>
  )
}

export function ChannelBadge({ channel, name, label }: { channel: string; name: string; label?: string }) {
  return <div className="flex flex-wrap items-center gap-1.5 text-caption text-text-3">
    <span className="inline-flex min-w-0 items-center gap-1.5 text-label font-semibold text-text" translate="no">
      <ChannelIcon channel={channel} size="md" />
      {name}
    </span>
    {label && <span>{label}</span>}
  </div>
}

export function PaymentOrder({ label, value, copyLabel }: { label: string; value: string; copyLabel: string }) {
  return <div className="mt-2 flex min-w-0 flex-wrap items-center justify-between gap-x-3 border-t border-dashed border-receipt-rule pt-1 md:mt-5 md:pt-2.5">
    <span className="text-caption text-text-2">{label}</span>
    <div className="min-w-0 text-end" translate="no"><CopyText value={value} label={copyLabel} /></div>
  </div>
}

export function PaymentAmount({ label, amount, original }: { label: ReactNode; amount: ReactNode; original?: ReactNode }) {
  return <div>
    <p className="text-caption text-text-2">{label}</p>
    <p className="mt-0.5 font-serif text-[clamp(2.75rem,5.3vw,4.75rem)] leading-none font-semibold tracking-[-0.04em] lining-nums tabular-nums">{amount}</p>
    {original && <p className="mt-1 text-caption text-text-2">{original}</p>}
  </div>
}

export function PaymentQR({ src, alt }: { src: string; alt: string }) {
  return <div className="w-[min(7rem,34vw)] rounded-control border border-receipt-rule bg-white p-2 md:w-36 md:p-2.5">
    <img width={128} height={128} className="aspect-square w-full object-contain" src={src} alt={alt} />
  </div>
}

export function PaymentResult({ kind, title, children }: { kind: 'paid' | 'expired' | 'refunded' | 'unavailable' | 'blocked'; title: ReactNode; children: ReactNode }) {
  const Icon = { paid: Check, expired: Clock, refunded: RotateCcw, unavailable: CircleAlert, blocked: ShieldX }[kind]
  const tone = { paid: 'bg-notice-success text-dot-ok', expired: 'bg-notice-warning text-dot-wait', refunded: 'bg-notice-info text-dot-info',
    unavailable: 'bg-notice-error text-danger', blocked: 'bg-notice-error text-danger' }[kind]
  return <div aria-live="polite" className="grid gap-2.5 md:gap-4">
    <h1 className="font-serif text-[clamp(1.5rem,2.6vw,2.25rem)] leading-tight">{title}</h1>
    <div className="flex items-start gap-3 border-t border-border pt-2.5 md:pt-4">
      <span className={cn('grid size-11 shrink-0 place-items-center rounded-full', tone)}><Icon className="size-5" aria-hidden="true" /></span>
      <div className="flex min-w-0 flex-col items-start gap-1 text-label text-text-2">{children}</div>
    </div>
  </div>
}
