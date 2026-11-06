import { Check, Copy } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

/**
 * Icon button that copies `value`. Feedback is the icon turning into a check
 * plus a polite live text — never a role=status, so pages keep one status.
 */
export function CopyButton({ value, label, className }: { value: string; label?: string; className?: string }) {
  const { t } = useTranslation()
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const timer = useRef<number>(undefined)
  useEffect(() => () => window.clearTimeout(timer.current), [])
  async function copy() {
    try {
      await navigator.clipboard.writeText(value)
      setState('copied')
    } catch {
      setState('failed')
    }
    window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => setState('idle'), 1800)
  }
  const name = label ?? t('ui.copy')
  return (
    <button
      type="button"
      onClick={() => { void copy() }}
      aria-label={name}
      title={name}
      className={cn(
        'relative grid size-11 shrink-0 place-items-center rounded-control text-text-3 transition-colors hover:bg-hover hover:text-text',
        state === 'copied' && 'text-dot-ok hover:text-dot-ok',
        className,
      )}
    >
      {state === 'copied' ? (
        <Check className="size-4" strokeWidth={2} aria-hidden="true" />
      ) : (
        <Copy className="size-4" strokeWidth={1.7} aria-hidden="true" />
      )}
      <span aria-live="polite" className="sr-only">
        {state === 'copied' ? t('ui.copied') : state === 'failed' ? t('ui.copyFailed') : ''}
      </span>
    </button>
  )
}

/** A monospace value with its copy button, for keys, IDs and URLs. */
export function CopyField({ value, label, className, children, testId }: { value: string; label?: string; className?: string; children?: React.ReactNode; testId?: string }) {
  return (
    <div className={cn('flex min-w-0 items-center gap-1 rounded-control bg-muted py-0 pl-3', className)}>
      <div data-testid={testId} className="min-w-0 flex-1 whitespace-pre-wrap py-2.5 font-mono text-[13px] leading-relaxed [overflow-wrap:anywhere]">{children ?? value}</div>
      <CopyButton value={value} label={label} />
    </div>
  )
}
