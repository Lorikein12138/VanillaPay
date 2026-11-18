import { Upload } from 'lucide-react'
import { useRef, useState, type ChangeEvent, type InputHTMLAttributes } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { buttonVariants } from './button'

/** The native input is the keyboard target. The visible button opens the same picker on click. */
export function FileInput({ className, onChange, disabled, ...props }: Omit<InputHTMLAttributes<HTMLInputElement>, 'type'>) {
  const { t } = useTranslation()
  const input = useRef<HTMLInputElement>(null)
  const [name, setName] = useState('')
  function change(event: ChangeEvent<HTMLInputElement>) {
    const files = event.currentTarget.files
    setName(files ? Array.from(files, (file) => file.name).join(', ') : '')
    onChange?.(event)
  }
  return (
    <div className={cn('flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1', className)}>
      <input ref={input} type="file" className="peer sr-only" disabled={disabled} onChange={change} {...props} />
      <button
        type="button"
        tabIndex={-1}
        aria-hidden="true"
        disabled={disabled}
        onMouseDown={(event) => event.preventDefault()}
        onClick={() => {
          input.current?.focus({ preventScroll: true })
          input.current?.click()
        }}
        className={cn(buttonVariants({ variant: 'secondary', size: 'sm' }),
          'peer-focus-visible:outline-2 peer-focus-visible:outline-offset-2 peer-focus-visible:outline-accent')}
      >
        <Upload aria-hidden="true" strokeWidth={1.8} />
        {t('ui.chooseFile')}
      </button>
      <span title={name || undefined} className={cn('min-w-0 flex-[1_1_8rem] truncate text-label', name ? 'text-text' : 'text-text-3')}>
        {name || t('ui.noFileChosen')}
      </span>
    </div>
  )
}
