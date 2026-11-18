import type { FormHTMLAttributes, ReactNode } from 'react'
import { TopBar } from '@/components/TopBar'
import { Card } from './layout'

export function AuthScreen({ title, description, footer, children }: {
  title: string; description: string; footer?: ReactNode; children: ReactNode
}) {
  return (
    <div className="flex min-h-dvh flex-col">
      <TopBar />
      <main id="main" className="mx-auto flex w-full max-w-[480px] flex-1 flex-col justify-center px-6 py-5 sm:py-10">
        <Card className="grid gap-4 border-0 bg-transparent p-0 sm:gap-6 sm:rounded-dialog sm:border sm:bg-surface sm:p-8 sm:shadow-dialog">
          <header className="grid gap-2">
            <h1 id="auth-screen-title" className="font-serif text-[2rem] leading-tight sm:text-title">{title}</h1>
            <p className="text-label leading-relaxed text-text-2">{description}</p>
          </header>
          {children}
        </Card>
      </main>
      {footer}
    </div>
  )
}

export function FormStack(props: FormHTMLAttributes<HTMLFormElement>) {
  return <form {...props} className="flex min-w-0 flex-col gap-5" />
}

export function FormHint({ children }: { children: ReactNode }) {
  return <p className="m-0 text-caption leading-relaxed text-text-3">{children}</p>
}
