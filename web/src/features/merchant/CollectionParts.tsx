import { Dialog } from 'radix-ui'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { FormError } from '@/components/FormError'
import { moduleError } from './collection-data'
import { orderButton } from './OrderParts'

export const modulePanel = 'rounded-card border-[0.5px] border-border bg-surface p-5 sm:p-6'
export function ModuleHeading({ title, children }: { title: string; children: ReactNode }) {
  return <div><h1 className="font-serif text-4xl font-[330]">{title}</h1><p className="mt-2 text-label text-text-2">{children}</p></div>
}
export function ModuleError({ error, retry }: { error: unknown; retry?: () => void }) {
  return <div className="space-y-3"><FormError>{moduleError(error)}</FormError>{retry && <button className={orderButton} onClick={retry}>重试</button>}</div>
}
export function ModuleDialog({ title, description, busy = false, onClose, children }: { title: string; description: string; busy?: boolean; onClose: () => void; children: ReactNode }) {
  return <Dialog.Root open onOpenChange={(open) => { if (!open && !busy) onClose() }}><Dialog.Portal>
    <Dialog.Overlay className="fixed inset-0 z-40 bg-black/40 backdrop-blur-[2px]" />
    <Dialog.Content className="fixed inset-x-4 top-[8dvh] z-50 mx-auto max-h-[84dvh] max-w-[520px] overflow-y-auto rounded-card border-[0.5px] border-border-strong bg-surface p-5 text-text shadow-xl sm:p-7" onEscapeKeyDown={(event) => { if (busy) event.preventDefault() }} onPointerDownOutside={(event) => { if (busy) event.preventDefault() }}>
      <Dialog.Title className="font-serif text-2xl">{title}</Dialog.Title><Dialog.Description className="mb-5 mt-3 text-label leading-relaxed text-text-2">{description}</Dialog.Description>
      {children}<div className="mt-5 flex justify-end"><button type="button" disabled={busy} className={orderButton} onClick={onClose}>关闭</button></div>
    </Dialog.Content>
  </Dialog.Portal></Dialog.Root>
}
// Local async actions never put security proofs or returned keys in mutation caches.
export function useLocalAction() {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>()
  const active = useRef(true), running = useRef(false)
  useEffect(() => { active.current = true; return () => { active.current = false } }, [])
  async function run(action: () => Promise<void>) {
    if (running.current) return
    running.current = true; setBusy(true); setError(undefined)
    try { await action() } catch (problem) { if (active.current) setError(problem) }
    finally { running.current = false; if (active.current) setBusy(false) }
  }
  return { busy, error, run, active }
}
export function CopyValue({ value, label = '复制' }: { value: string; label?: string }) {
  const [status, setStatus] = useState('')
  return <span className="inline-flex flex-wrap items-center gap-2"><button type="button" className={orderButton} onClick={() => { void navigator.clipboard.writeText(value).then(() => setStatus('已复制'), () => setStatus('复制失败，请手动复制')) }}>{label}</button><span role="status" className="text-caption text-text-2">{status}</span></span>
}
