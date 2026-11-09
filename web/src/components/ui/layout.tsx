import { createLink } from '@tanstack/react-router'
import { ArrowLeft } from 'lucide-react'
import { Fragment, type AnchorHTMLAttributes, type HTMLAttributes, type ReactNode, type Ref } from 'react'
import { cn } from '@/lib/utils'

/** Page container shared by every merchant page. */
export function Page({ className, children, ...props }: HTMLAttributes<HTMLDivElement> & { children: ReactNode }) {
  return (
    <div className={cn('mx-auto flex w-full max-w-[1112px] flex-col gap-10', className)} {...props}>
      {children}
    </div>
  )
}

/** Serif page title with optional lead text, back link and right-aligned actions. */
export function PageHeader({
  title,
  description,
  back,
  meta,
  actions,
}: {
  title: ReactNode
  description?: ReactNode
  back?: ReactNode
  /** Small line under the title, e.g. an order number. */
  meta?: ReactNode
  actions?: ReactNode
}) {
  return (
    <header className="flex flex-col gap-4">
      {back}
      <div className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
        <div className="flex min-w-0 flex-col gap-2">
          <h1 className="m-0 font-serif text-[2rem] font-[330] leading-[1.2] sm:text-[2.25rem]">{title}</h1>
          {description && <p className="m-0 max-w-[62ch] text-label leading-relaxed text-text-2">{description}</p>}
          {meta}
        </div>
        {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
      </div>
    </header>
  )
}

function BackAnchor({ children, className, ref, ...props }: AnchorHTMLAttributes<HTMLAnchorElement> & { ref?: Ref<HTMLAnchorElement> }) {
  return (
    <a
      ref={ref}
      {...props}
      className={cn('-ml-1 inline-flex min-h-11 w-fit items-center gap-1.5 rounded-control px-1 text-label text-text-2 transition-colors hover:text-text', className)}
    >
      <ArrowLeft className="size-4" strokeWidth={1.8} aria-hidden="true" />
      {children}
    </a>
  )
}

/** "← Back to …" link above a page title; takes the same props as `<Link>`. */
export const BackLink = createLink(BackAnchor)

/** White panel with the template's hairline border and 16px radius. */
export function Card({ className, children, ...props }: HTMLAttributes<HTMLElement> & { children: ReactNode }) {
  return (
    <section className={cn('min-w-0 rounded-card border-[0.5px] border-border bg-surface p-5 sm:p-7', className)} {...props}>
      {children}
    </section>
  )
}

/** Card/section heading row: serif title, optional description and actions. */
export function SectionHeader({
  title,
  description,
  actions,
  as: Heading = 'h2',
  className,
}: {
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  as?: 'h2' | 'h3'
  className?: string
}) {
  return (
    <div className={cn('mb-5 flex flex-wrap items-start justify-between gap-x-4 gap-y-3', className)}>
      <div className="flex min-w-0 flex-col gap-1.5">
        <Heading className="m-0 font-serif text-heading">{title}</Heading>
        {description && <p className="m-0 max-w-[62ch] text-label leading-relaxed text-text-2">{description}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </div>
  )
}

/** A smaller, sans-serif heading within a card. */
export function SubsectionTitle({ className, children }: { className?: string; children: ReactNode }) {
  return <h3 className={cn('m-0 font-sans text-body font-medium text-text', className)}>{children}</h3>
}

/** Muted centered message for empty lists. */
export function EmptyState({ icon, title, children }: { icon?: ReactNode; title: ReactNode; children?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-2 border-t-[0.5px] border-border px-4 py-14 text-center">
      {icon && <span className="mb-1 text-text-3 [&_svg]:size-6">{icon}</span>}
      <p className="m-0 text-label text-text-2">{title}</p>
      {children && <div className="text-caption text-text-3">{children}</div>}
    </div>
  )
}

export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden="true" className={cn('animate-pulse rounded-control bg-muted motion-reduce:animate-none', className)} />
}

/** Monospace identifier; wraps only where it must (narrow screens). */
export function Mono({ className, children }: { className?: string; children: ReactNode }) {
  return <span className={cn('font-mono text-[13px] [overflow-wrap:anywhere]', className)}>{children}</span>
}

/** Prefer hyphen breaks without leaving short prefixes or suffixes on a line of their own. */
export function Identifier({ value, className }: { value: string; className?: string }) {
  const groups: string[] = []
  let pending = ''
  for (const part of value.match(/[^-]*-+|[^-]+/g) ?? []) {
    pending += part
    if (pending.length >= 4) {
      groups.push(pending)
      pending = ''
    }
  }
  if (pending) groups.push((groups.pop() ?? '') + pending)

  return (
    <Mono className={cn('min-w-0 [text-decoration:inherit]', className)}>
      {groups.map((group, index) => (
        <Fragment key={index}>
          {index > 0 && <wbr />}
          <span className="inline-block max-w-full text-balance [text-decoration:inherit]">{group}</span>
        </Fragment>
      ))}
    </Mono>
  )
}
