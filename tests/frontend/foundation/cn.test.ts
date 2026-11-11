import { describe, expect, it } from 'vitest'
import { cn } from '@/lib/utils'

describe('cn', () => {
  it('keeps a design-token font size next to a token text color', () => {
    expect(cn('text-caption', 'text-text-3')).toBe('text-caption text-text-3')
    expect(cn('text-label text-text-2', 'text-accent-text')).toBe('text-label text-accent-text')
  })

  it('lets later size, radius and shadow tokens override earlier ones', () => {
    expect(cn('text-xl', 'text-lg')).toBe('text-lg')
    expect(cn('text-label', 'text-body')).toBe('text-body')
    expect(cn('text-label', 'text-[13px]')).toBe('text-[13px]')
    expect(cn('rounded-card', 'rounded-lg')).toBe('rounded-lg')
    expect(cn('rounded-lg', 'rounded-control')).toBe('rounded-control')
    expect(cn('shadow-dialog', 'shadow-pop')).toBe('shadow-pop')
  })
})
