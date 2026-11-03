import type { ReactNode } from 'react'

/** Centered message block used for 404 and unexpected route errors. */
export function ErrorState({
  code,
  title,
  body,
  action,
}: {
  code?: string
  title: string
  body: string
  action?: ReactNode
}) {
  return (
    <section className="mx-auto flex max-w-[560px] flex-col items-start gap-4 px-4 py-24 sm:px-6">
      {code ? <span className="font-serif text-figure text-accent-text tabular-nums">{code}</span> : null}
      <h1 className="m-0 font-serif text-title font-medium">{title}</h1>
      <p className="m-0 text-text-2">{body}</p>
      {action}
    </section>
  )
}
