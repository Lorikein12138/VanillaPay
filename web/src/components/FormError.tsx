import type { ReactNode } from 'react'
import { Notice } from '@/components/ui/notice'

export function FormError({ children, id }: { children: ReactNode; id?: string }) {
  return <Notice id={id} tone="error">{children}</Notice>
}
