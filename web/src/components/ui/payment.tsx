import type { ReactNode } from 'react'
import { Check, ChevronRight, CircleAlert, Clock, RotateCcw, ShieldX } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button, type ButtonProps } from './button'
import { channelPaths } from './channel-paths'
import { CopyButton } from './copy-button'

export function PaymentShell({ controls, receiptLabel, summary, footer, order, children }: {
  controls: ReactNode; receiptLabel: ReactNode; summary?: ReactNode; footer: ReactNode; order?: ReactNode; children: ReactNode
}) {
  const { t } = useTranslation()
  return <div className="flex min-h-dvh flex-col bg-stage">
    <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:z-10 focus:rounded-control focus:bg-surface focus:p-3">{t('common.skipToContent')}</a>
    <header className="border-b border-border bg-bg px-3 py-1 sm:px-6">
      <div className="mx-auto flex max-w-[1120px] flex-wrap items-center justify-end gap-0.5">{controls}</div>
    </header>
    <main id="main" tabIndex={-1} className="grid flex-1 place-items-center px-3 py-3 max-[20rem]:px-1.5 sm:px-6 sm:py-6">
      <article className="receipt-paper w-full min-w-0 max-w-[492px] px-4 pt-3 pb-3.5 [overflow-wrap:anywhere] max-[20rem]:px-2 sm:px-7 sm:pt-5 sm:pb-4">
        <header className="flex flex-wrap items-baseline justify-between gap-x-2 border-b border-receipt-rule pb-1.5 sm:pb-2">
          <span className="text-body font-bold tracking-tight sm:text-lg" translate="no">VanillaPay</span>
          <span className="text-caption text-text-2">{receiptLabel}</span>
        </header>
        {summary}
        <div className="my-3 flex min-w-0 flex-col gap-2.5 border-y border-dashed border-receipt-rule py-3 sm:my-4 sm:py-3.5">
          {children}
        </div>
        <footer>
          {order}
          <p className="text-end text-caption text-text-3">{footer}</p>
        </footer>
      </article>
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
  return <dl className="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-3">
    <dt className="text-caption text-text-2">{label}</dt>
    <dd className="flex min-w-0 items-center justify-end gap-1 text-end" translate="no">
      <span className="min-w-0 font-mono text-caption">{value}</span>
      <CopyButton value={value} label={copyLabel} />
    </dd>
  </dl>
}

export function PaymentAmount({ label, amount, original }: { label: ReactNode; amount: ReactNode; original?: ReactNode }) {
  return <div>
    <p className="text-caption text-text-2">{label}</p>
    <p className="mt-0.5 font-serif text-[clamp(2.75rem,5.3vw,4rem)] leading-none font-semibold tracking-[-0.045em] lining-nums tabular-nums">{amount}</p>
    {original && <p className="mt-1 text-caption text-text-2">{original}</p>}
  </div>
}

export function PaymentQR({ src, alt }: { src: string; alt: string }) {
  return <div className="w-[min(8.25rem,35vw)] rounded-control border border-receipt-rule bg-white p-2 sm:p-2.5">
    <img width={128} height={128} className="aspect-square w-full object-contain" src={src} alt={alt} />
  </div>
}

export function PaymentResult({ kind, title, children, heading: Heading = 'h1' }: {
  kind: 'paid' | 'expired' | 'refunded' | 'unavailable' | 'blocked'; title: ReactNode; children: ReactNode; heading?: 'h1' | 'h2'
}) {
  const Icon = { paid: Check, expired: Clock, refunded: RotateCcw, unavailable: CircleAlert, blocked: ShieldX }[kind]
  const tone = { paid: 'bg-notice-success text-dot-ok', expired: 'bg-notice-warning text-dot-wait', refunded: 'bg-notice-info text-dot-info',
    unavailable: 'bg-notice-error text-danger', blocked: 'bg-notice-error text-danger' }[kind]
  return <div className="grid gap-2.5">
    <Heading className="font-serif text-[clamp(1.375rem,2.6vw,1.75rem)] leading-tight text-balance">{title}</Heading>
    <div className="flex items-start gap-3">
      <span className={cn('grid size-11 shrink-0 place-items-center rounded-full', tone)}><Icon className="size-5" aria-hidden="true" /></span>
      <div className="flex min-w-0 flex-col items-start gap-1 text-label text-text-2">{children}</div>
    </div>
  </div>
}
