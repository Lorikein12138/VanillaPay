import { Upload } from 'lucide-react'
import { useRef, useState, type ChangeEvent, type InputHTMLAttributes } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { buttonVariants } from './button'

/**
 * Native file input (kept for keyboard, labels and tests) drawn as a
 * "选择文件" button plus the chosen file name. Give it `id` and point a
 * `<Field htmlFor>` at it; reset it by changing its `key`.
 */
export function FileInput({ className, onChange, disabled, ...props }: Omit<InputHTMLAttributes<HTMLInputElement>, 'type'>) {
  const { t } = useTranslation()
  const input = useRef<HTMLInputElement>(null)
  const [name, setName] = useState('')
  function change(event: ChangeEvent<HTMLInputElement>) {
    const files = event.target.files
    setName(files && files.length ? Array.from(files, (file) => file.name).join(', ') : '')
    onChange?.(event)
  }
  return (
    <div className={cn('flex min-w-0 items-center gap-3', className)}>
      <input ref={input} type="file" className="peer sr-only" disabled={disabled} onChange={change} {...props} />
      <button
        type="button"
        tabIndex={-1}
        aria-hidden="true"
        disabled={disabled}
        onClick={() => input.current?.click()}
        className={cn(buttonVariants({ variant: 'secondary' }), 'peer-focus-visible:shadow-[0_0_0_1px_var(--color-accent),0_0_0_4px_var(--color-focus-ring)]')}
      >
        <Upload aria-hidden="true" strokeWidth={1.8} />
        {t('ui.chooseFile')}
      </button>
      <span className={cn('min-w-0 truncate text-label', name ? 'text-text' : 'text-text-3')}>{name || t('ui.noFileChosen')}</span>
    </div>
  )
}
