import { CircleAlert, CircleCheck, Info, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

const tones = {
  success: { icon: CircleCheck, color: 'text-dot-ok', role: 'status' },
  info: { icon: Info, color: 'text-dot-info', role: 'status' },
  error: { icon: CircleAlert, color: 'text-danger', role: 'alert' },
} as const

/**
 * Inline result message for a page or card ("补单成功" / "已保存"). Success and
 * info are a polite status, errors an alert. Pass `onDismiss` for a close
 * button; render it only while there is something to say.
 */
export function Notice({
  tone = 'success',
  id,
  title,
  children,
  action,
  onDismiss,
  className,
}: {
  id?: string
  tone?: keyof typeof tones
  title?: ReactNode
  children: ReactNode
  action?: ReactNode
  onDismiss?: () => void
  className?: string
}) {
  const { t } = useTranslation()
  const { icon: Icon, color, role } = tones[tone]
  return (
    <div
      id={id}
      role={role}
      aria-live="polite"
      className={cn(
        'flex items-start gap-3 rounded-control border-[0.5px] border-border bg-surface py-3 pl-4 pr-2 text-label',
        tone === 'error' && 'border-danger/40',
        className,
      )}
    >
      <Icon className={cn('mt-0.5 size-[18px] shrink-0', color)} strokeWidth={1.7} aria-hidden="true" />
      <div className="min-w-0 flex-1 py-px leading-relaxed">
        {title && <p className="m-0 font-medium text-text">{title}</p>}
        <div className={cn(title ? 'text-text-2' : 'text-text')}>{children}</div>
        {action && <div className="mt-2">{action}</div>}
      </div>
      {onDismiss && (
        <button
          type="button"
          onClick={onDismiss}
          aria-label={t('ui.dismiss')}
          className="-my-2 grid size-11 shrink-0 place-items-center rounded-control text-text-3 transition-colors hover:bg-hover hover:text-text"
        >
          <X className="size-4" strokeWidth={1.8} aria-hidden="true" />
        </button>
      )}
    </div>
  )
}
