import { clsx, type ClassValue } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

// Register the design tokens from styles.css. Without them tailwind-merge reads
// `text-caption` as a text color and drops it next to `text-text-3`, and never
// lets `rounded-lg` override `rounded-card`.
const twMerge = extendTailwindMerge({
  extend: {
    theme: {
      text: ['display', 'title', 'heading', 'figure', 'body', 'label', 'caption'],
      radius: ['control', 'card', 'dialog'],
      shadow: ['dialog', 'pop'],
    },
  },
})

export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs))
}
