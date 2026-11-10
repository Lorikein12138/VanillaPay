import { X } from 'lucide-react'
import { Dialog } from 'radix-ui'
import { useState, type ReactNode, type RefObject } from 'react'
import { DropdownMenuContainer } from './dropdown-menu'
import { useTranslation } from 'react-i18next'

/** Mobile navigation shares Radix focus trapping, Escape and focus restoration. */
export function Drawer({ open, onClose, title, children, returnFocus }: { open: boolean; onClose: () => void; title: string; children: ReactNode; returnFocus: RefObject<HTMLButtonElement | null> }) {
  const { t } = useTranslation()
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return <Dialog.Root open={open} onOpenChange={(value) => { if (!value) onClose() }}><Dialog.Portal>
    <Dialog.Overlay className="fixed inset-0 z-40 bg-overlay backdrop-blur-[2px]" />
    <Dialog.Content ref={setContainer} onCloseAutoFocus={(event) => { event.preventDefault(); returnFocus.current?.focus() }} className="fixed inset-y-0 left-0 z-50 flex w-[min(320px,calc(100vw-32px))] flex-col gap-6 overflow-y-auto bg-sidebar p-5 text-text shadow-dialog">
      <div className="flex items-center justify-between gap-3"><Dialog.Title className="font-serif text-heading">{title}</Dialog.Title><Dialog.Close aria-label={t('ui.close')} className="grid size-11 place-items-center rounded-control text-text-2 hover:bg-hover"><X className="size-5" aria-hidden="true" /></Dialog.Close></div>
      <Dialog.Description className="sr-only">{title}</Dialog.Description>
      <DropdownMenuContainer.Provider value={container}>{children}</DropdownMenuContainer.Provider>
    </Dialog.Content>
  </Dialog.Portal></Dialog.Root>
}
