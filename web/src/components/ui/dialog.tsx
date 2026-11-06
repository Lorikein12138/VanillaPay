import { X } from 'lucide-react'
import { Dialog as Primitive } from 'radix-ui'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

const widths = { sm: 'max-w-[440px]', md: 'max-w-[520px]', lg: 'max-w-[640px]' } as const

/**
 * Modal with a fixed header and footer and a scrolling body, so tall content
 * (QR codes, PEM keys) never pushes the actions off-screen. While `busy`,
 * Escape, outside clicks and the close button are ignored.
 */
export function Dialog({
  open = true,
  onClose,
  title,
  description,
  footer,
  busy = false,
  size = 'md',
  children,
}: {
  open?: boolean
  onClose: () => void
  title: ReactNode
  description?: ReactNode
  /** Action buttons; the primary action goes last so it sits on the right. */
  footer?: ReactNode
  busy?: boolean
  size?: keyof typeof widths
  children?: ReactNode
}) {
  const { t } = useTranslation()
  return (
    <Primitive.Root open={open} onOpenChange={(next) => { if (!next && !busy) onClose() }}>
      <Primitive.Portal>
        <Primitive.Overlay className="vp-overlay fixed inset-0 z-40 bg-overlay backdrop-blur-[2px]" />
        <div className="pointer-events-none fixed inset-0 z-50 flex items-end justify-center p-0 sm:items-center sm:p-6">
          <Primitive.Content
            onEscapeKeyDown={(event) => { if (busy) event.preventDefault() }}
            onPointerDownOutside={(event) => { if (busy) event.preventDefault() }}
            onInteractOutside={(event) => { if (busy) event.preventDefault() }}
            className={cn(
              'vp-dialog pointer-events-auto flex max-h-[calc(100dvh-12px)] w-full flex-col overflow-hidden rounded-t-dialog border-[0.5px] border-border-strong bg-surface text-text shadow-dialog focus:outline-none sm:max-h-[calc(100dvh-48px)] sm:rounded-dialog',
              widths[size],
            )}
          >
            <div className="flex items-start gap-4 px-5 pb-1 pt-5 sm:px-7 sm:pt-6">
              <div className="min-w-0 flex-1 pt-1.5">
                <Primitive.Title className="m-0 font-serif text-heading">{title}</Primitive.Title>
                {description ? (
                  <Primitive.Description className="m-0 mt-2 text-label leading-relaxed text-text-2">{description}</Primitive.Description>
                ) : (
                  <Primitive.Description className="sr-only">{title}</Primitive.Description>
                )}
              </div>
              <Primitive.Close
                disabled={busy}
                aria-label={t('ui.close')}
                className="-mr-2 -mt-0.5 grid size-11 shrink-0 place-items-center rounded-control text-text-3 transition-colors hover:bg-hover hover:text-text disabled:opacity-40"
              >
                <X className="size-[18px]" strokeWidth={1.8} aria-hidden="true" />
              </Primitive.Close>
            </div>
            {children != null && <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-4 sm:px-7">{children}</div>}
            {footer && (
              <div className="flex flex-col-reverse gap-2 border-t-[0.5px] border-border px-5 pb-[max(1.25rem,env(safe-area-inset-bottom))] pt-4 sm:flex-row sm:justify-end sm:px-7 sm:pb-5 [&>*]:w-full sm:[&>*]:w-auto">
                {footer}
              </div>
            )}
          </Primitive.Content>
        </div>
      </Primitive.Portal>
    </Primitive.Root>
  )
}
