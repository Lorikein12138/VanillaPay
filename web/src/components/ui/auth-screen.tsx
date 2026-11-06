import type { FormHTMLAttributes, ReactNode } from 'react'
import { TopBar } from '@/components/TopBar'
import { Card, PageHeader } from './layout'

export function AuthScreen({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return <div className="flex min-h-dvh flex-col"><TopBar /><main id="main" className="mx-auto flex w-full max-w-xl flex-1 flex-col justify-center gap-6 px-4 py-12 sm:px-8"><PageHeader title={title} description={description} /><Card>{children}</Card></main></div>
}

export function FormStack(props: FormHTMLAttributes<HTMLFormElement>) {
  return <form {...props} className="flex min-w-0 flex-col gap-5" />
}

export function FormHint({ children }: { children: ReactNode }) {
  return <p className="m-0 text-caption leading-relaxed text-text-3">{children}</p>
}
