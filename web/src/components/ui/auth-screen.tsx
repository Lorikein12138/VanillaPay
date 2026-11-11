import type { FormHTMLAttributes, ReactNode } from 'react'
import { TopBar } from '@/components/TopBar'
import { Card } from './layout'

export function AuthScreen({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return <div className="flex min-h-dvh flex-col"><TopBar /><main id="main" className="mx-auto flex w-full max-w-[480px] flex-1 flex-col justify-center gap-8 px-4 pb-16 pt-8 sm:px-6">
    <header className="flex flex-col items-center gap-3 text-center">
      <h1 className="m-0 font-serif text-[2.25rem] font-normal leading-[1.1] tracking-[-0.015em] sm:text-[2.75rem]">{title}</h1>
      <p className="m-0 max-w-[40ch] text-[15px] leading-relaxed text-text-2">{description}</p>
    </header>
    <Card className="rounded-[24px] p-6 shadow-[0_1px_2px_rgba(20,20,19,0.03)] sm:p-8">{children}</Card>
  </main></div>
}

export function FormStack(props: FormHTMLAttributes<HTMLFormElement>) {
  return <form {...props} className="flex min-w-0 flex-col gap-5" />
}

export function FormHint({ children }: { children: ReactNode }) {
  return <p className="m-0 text-caption leading-relaxed text-text-3">{children}</p>
}
