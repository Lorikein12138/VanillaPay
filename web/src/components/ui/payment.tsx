import type { ReactNode } from 'react'
import { Check, Clock, RotateCcw, Store } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button, type ButtonProps } from './button'
import { channelPaths } from './channel-paths'
import { CopyText } from './copy-button'
import { Card } from './layout'

/** Public payment header keeps controls visible while long shop names truncate. */
export function PaymentShell({ merchant, controls, footer, children }: { merchant: ReactNode; controls: ReactNode; footer: ReactNode; children: ReactNode }) {
  return <div className="flex min-h-dvh flex-col bg-bg">
    <header className="border-b-[0.5px] border-border px-4 py-3">
      <div className="mx-auto flex max-w-[960px] items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-1.5"><Store className="size-5 shrink-0 text-accent-text" strokeWidth={1.7} aria-hidden="true" /><span className="min-w-0 truncate font-serif text-xl tracking-[-0.01em]">{merchant}</span></div>
        <div className="flex shrink-0 items-center gap-0.5">{controls}</div>
      </div>
    </header>
    <main className="mx-auto flex w-full max-w-[520px] flex-1 flex-col justify-center px-4 py-8 sm:py-14"><Card className="flex flex-col gap-6 rounded-[24px] shadow-[0_1px_2px_rgba(20,20,19,0.03),0_20px_48px_-28px_rgba(20,20,19,0.12)] sm:p-9">{children}</Card></main>
    <footer className="px-4 pb-6 text-center text-caption text-text-3">{footer}</footer>
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
  alipay: '[--channel-color:#1677ff] dark:[--channel-color:#6ba6ff]',
  wxpay: '[--channel-color:#07a855] dark:[--channel-color:#47d68a]',
  qqpay: '[--channel-color:#008cdb] dark:[--channel-color:#58bfff]',
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
      'inline-grid shrink-0 place-items-center bg-[var(--channel-color)] text-white',
      size === 'md' ? 'size-6 rounded-[6px]' : 'size-4 rounded-[4px]',
      brand ? channelColors[brand] : '[--channel-color:var(--vp-text-3)]',
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
      'h-14 min-w-0 justify-start gap-2.5 rounded-xl border-border bg-field px-2.5 hover:border-border-strong hover:bg-hover sm:px-4',
      'focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent disabled:cursor-not-allowed',
      selected && 'border-accent bg-accent-soft ring-1 ring-accent/30 hover:bg-accent-soft', className,
    )}>
      <ChannelIcon channel={channel} size="md" />
      <span className="min-w-0 truncate">{children}</span>
    </Button>
  )
}

export function ChannelGroup({ label, count, children }: { label: string; count: number; children: ReactNode }) {
  return <div role="group" aria-label={label} className="flex flex-col gap-2.5">
    <p className="text-caption text-text-3">{label}</p>
    <div className={cn('grid gap-2.5', count === 2 ? 'grid-cols-2' : count > 2 ? 'grid-cols-1 sm:grid-cols-3' : 'grid-cols-1')}>{children}</div>
  </div>
}

export function ChannelBadge({ channel, name, label }: { channel: string; name: string; label?: string }) {
  return <div className="flex flex-wrap items-center justify-center gap-2 text-caption text-text-3">
    <span className="inline-flex items-center gap-2 whitespace-nowrap rounded-full bg-muted px-3 py-1.5 text-label text-text-2">
      <ChannelIcon channel={channel} />
      {name}
    </span>
    {label && <span>{label}</span>}
  </div>
}

export function PaymentOrder({ label, value, copyLabel }: { label: string; value: string; copyLabel: string }) {
  return <div className="flex min-w-0 items-center justify-between gap-3 border-t-[0.5px] border-border pt-4">
    <span className="shrink-0 text-caption text-text-3">{label}</span>
    <div className="min-w-0 text-right"><CopyText value={value} label={copyLabel} /></div>
  </div>
}

export function PaymentAmount({ name, label, amount, original }: { name: ReactNode; label: ReactNode; amount: ReactNode; original?: ReactNode }) {
  return <div className="flex flex-col items-center gap-2 text-center">
    <h1 className="m-0 break-words font-serif text-heading tracking-[-0.005em]">{name}</h1>
    <p className="text-label text-text-2">{label}</p>
    <div className="flex flex-wrap items-baseline justify-center gap-3"><span className="font-serif text-[3.25rem] leading-tight tracking-[-0.02em] tabular-nums">{amount}</span>{original && <del className="text-label text-text-3">{original}</del>}</div>
  </div>
}

export function PaymentQR({ src, alt }: { src: string; alt: string }) {
  return <div className="mx-auto w-full max-w-[248px] rounded-2xl bg-white p-4 shadow-[0_0_0_0.5px_rgba(20,20,19,0.1)]"><img className="aspect-square w-full object-contain" src={src} alt={alt} /></div>
}

export function PaymentResult({ kind, title, children }: { kind: 'paid' | 'expired' | 'refunded' | 'unavailable'; title: ReactNode; children: ReactNode }) {
  const Icon = kind === 'paid' ? Check : kind === 'refunded' ? RotateCcw : Clock
  return <div className="flex flex-col items-center gap-4 pt-3 text-center">
    <span className={kind === 'paid' ? 'rounded-full bg-dot-ok/10 p-4 text-dot-ok' : 'rounded-full bg-muted p-4 text-text-2'}><Icon className="size-7" aria-hidden="true" /></span>
    <h1 className="font-serif text-[1.625rem] leading-[1.25] tracking-[-0.01em]">{title}</h1><div className="flex w-full flex-col items-center gap-4 text-label text-text-2">{children}</div>
  </div>
}
