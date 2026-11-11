import { X } from 'lucide-react'
import { Dialog } from 'radix-ui'
import { useRef, useState, type ReactNode, type RefObject } from 'react'
import { focusQuietly } from '@/lib/quiet-focus'
import { DropdownMenuContainer } from './dropdown-menu'
import { useTranslation } from 'react-i18next'

/** Mobile navigation shares Radix focus trapping, Escape and focus restoration. */
export function Drawer({ open, onClose, title, children, returnFocus }: { open: boolean; onClose: () => void; title: string; children: ReactNode; returnFocus: RefObject<HTMLButtonElement | null> }) {
  const { t } = useTranslation()
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const pointerClose = useRef(false)
  const markPointer = () => { pointerClose.current = true }
  return <Dialog.Root open={open} onOpenChange={(value) => { if (!value) onClose() }}><Dialog.Portal>
    <Dialog.Overlay onPointerDown={markPointer} className="vp-overlay fixed inset-0 z-40 bg-overlay" />
    <Dialog.Content ref={setContainer} onPointerDown={markPointer} onKeyDown={() => { pointerClose.current = false }} onOpenAutoFocus={(event) => {
      // Open on the panel itself instead of ringing the close button.
      event.preventDefault()
      ;(event.target as HTMLElement).focus({ preventScroll: true })
    }} onCloseAutoFocus={(event) => {
      event.preventDefault()
      // Mouse closes return focus without the keyboard ring; Escape keeps it.
      if (pointerClose.current) focusQuietly(returnFocus.current)
      else returnFocus.current?.focus()
      pointerClose.current = false
    }} className="fixed inset-y-0 left-0 z-50 flex w-[min(320px,calc(100vw-32px))] flex-col gap-6 overflow-y-auto bg-sidebar focus:outline-none px-4 py-5 text-text shadow-dialog">
      <div className="flex items-center justify-between gap-3"><Dialog.Title className="font-serif text-heading">{title}</Dialog.Title><Dialog.Close aria-label={t('ui.close')} className="grid size-11 place-items-center rounded-control text-text-2 hover:bg-hover"><X className="size-5" aria-hidden="true" /></Dialog.Close></div>
      <Dialog.Description className="sr-only">{title}</Dialog.Description>
      <DropdownMenuContainer.Provider value={container}>{children}</DropdownMenuContainer.Provider>
    </Dialog.Content>
  </Dialog.Portal></Dialog.Root>
}
