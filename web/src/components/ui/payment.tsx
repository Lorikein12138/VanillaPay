import type { ReactNode } from 'react'
import { Check, Clock, RotateCcw, Store } from 'lucide-react'
import { Card } from './layout'

/** Public payment shell; controls wrap independently of a long shop name. */
export function PaymentShell({ merchant, controls, footer, children }: { merchant: ReactNode; controls: ReactNode; footer: ReactNode; children: ReactNode }) {
  return <div className="flex min-h-dvh flex-col bg-bg">
    <header className="border-b-[0.5px] border-border bg-surface px-5 py-3">
      <div className="mx-auto flex max-w-[960px] flex-wrap items-center justify-between gap-x-5 gap-y-2">
        <div className="flex min-w-0 items-center gap-2.5"><Store className="size-5 shrink-0 text-primary" aria-hidden="true" /><span className="min-w-0 font-serif text-xl [overflow-wrap:anywhere]">{merchant}</span></div>
        <div className="flex shrink-0 items-center gap-2">{controls}</div>
      </div>
    </header>
    <main className="mx-auto flex w-full max-w-[560px] flex-1 flex-col justify-center px-4 py-8 sm:py-12"><Card className="flex flex-col gap-6">{children}</Card></main>
    <footer className="px-4 pb-6 text-center text-caption text-text-3">{footer}</footer>
  </div>
}

export function PaymentAmount({ name, label, amount, original }: { name: ReactNode; label: ReactNode; amount: ReactNode; original?: ReactNode }) {
  return <div className="flex flex-col items-center gap-2 text-center">
    <h1 className="m-0 break-words font-serif text-heading">{name}</h1>
    <p className="text-label text-text-2">{label}</p>
    <div className="flex flex-wrap items-baseline justify-center gap-3"><span className="font-serif text-[3rem] leading-tight tabular-nums">{amount}</span>{original && <del className="text-label text-text-3">{original}</del>}</div>
  </div>
}

export function PaymentQR({ src, alt }: { src: string; alt: string }) {
  return <div className="mx-auto w-full max-w-[248px] rounded-card border-[0.5px] border-border bg-white p-4"><img className="aspect-square w-full object-contain" src={src} alt={alt} /></div>
}

export function PaymentResult({ kind, title, children }: { kind: 'paid' | 'expired' | 'refunded' | 'unavailable'; title: ReactNode; children: ReactNode }) {
  const Icon = kind === 'paid' ? Check : kind === 'refunded' ? RotateCcw : Clock
  return <div className="flex flex-col items-center gap-4 py-7 text-center">
    <span className={kind === 'paid' ? 'rounded-full bg-dot-ok/10 p-4 text-dot-ok' : 'rounded-full bg-muted p-4 text-text-2'}><Icon className="size-7" aria-hidden="true" /></span>
    <h1 className="font-serif text-heading">{title}</h1><div className="flex w-full flex-col items-center gap-4 text-label text-text-2">{children}</div>
  </div>
}
