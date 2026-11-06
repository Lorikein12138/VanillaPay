import { cva, type VariantProps } from 'class-variance-authority'
import { LoaderCircle } from 'lucide-react'
import type { ComponentPropsWithRef } from 'react'
import { cn } from '@/lib/utils'

/**
 * Shared button styles. 44px default height keeps every target touch-sized;
 * `sm` uses compact horizontal padding while retaining the touch target.
 */
export const buttonVariants = cva(
  'inline-flex shrink-0 select-none items-center justify-center gap-2 whitespace-nowrap rounded-control text-label font-medium transition-[background-color,color,border-color,opacity] duration-150 disabled:pointer-events-none disabled:opacity-45 [&_svg]:size-4 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        primary: 'bg-primary text-primary-fg hover:bg-primary/85',
        secondary: 'border-[0.5px] border-border-strong bg-surface text-text hover:bg-hover',
        ghost: 'text-text-2 hover:bg-hover hover:text-text',
        danger: 'text-danger hover:bg-danger/10',
        link: 'h-auto min-h-11 px-0 font-normal text-accent-text underline-offset-4 hover:text-accent-text-hover hover:underline',
      },
      size: {
        md: 'h-11 px-[18px]',
        sm: 'h-11 px-3',
        icon: 'size-11',
      },
    },
    compoundVariants: [{ variant: 'link', className: 'h-auto px-0' }],
    defaultVariants: { variant: 'secondary', size: 'md' },
  },
)

export type ButtonProps = ComponentPropsWithRef<'button'> &
  VariantProps<typeof buttonVariants> & {
    /** Shows a spinner and disables the button while keeping its label for screen readers. */
    loading?: boolean
  }

export function Button({ className, variant, size, loading = false, disabled, type = 'button', children, ...props }: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={cn(buttonVariants({ variant, size }), className)}
      {...props}
    >
      {loading && <LoaderCircle className="animate-spin motion-reduce:animate-none" aria-hidden="true" />}
      {children}
    </button>
  )
}
