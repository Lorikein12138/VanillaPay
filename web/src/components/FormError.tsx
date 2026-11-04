import { CircleAlert } from 'lucide-react'
import type { ReactNode } from 'react'

export function FormError({ children, id }: { children: ReactNode; id?: string }) {
  return (
    <p id={id} role="alert" aria-live="polite" className="m-0 flex items-start gap-2 text-label text-danger">
      <CircleAlert className="mt-0.5 size-4 shrink-0" strokeWidth={1.6} aria-hidden="true" />
      <span>{children}</span>
    </p>
  )
}
