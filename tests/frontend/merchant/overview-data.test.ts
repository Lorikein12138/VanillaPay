import { describe, expect, it } from 'vitest'
import { amountChange, money, previousRange, statsRange, successRate } from '@/features/merchant/overview-data'

describe('overview calendar ranges', () => {
  it.each([
    [1, '2026-11-04T16:00:00.000Z'],
    [7, '2026-10-29T16:00:00.000Z'],
    [30, '2026-10-06T16:00:00.000Z'],
  ] as const)('uses Shanghai local midnight for %s days', (days, start) => {
    const range = statsRange(new Date('2026-11-05T10:20:00+08:00'), days, 'Asia/Shanghai')
    expect(range).toEqual({ start, end: '2026-11-05T16:00:00.000Z', timezone: 'Asia/Shanghai' })
    const previous = previousRange(range)
    expect(previous.end).toBe(range.start)
    expect(Date.parse(previous.end) - Date.parse(previous.start)).toBe(Date.parse(range.end) - Date.parse(range.start))
  })
  it.each([
    ['2026-03-08T12:00:00-04:00', '2026-03-08T05:00:00.000Z', '2026-03-09T04:00:00.000Z', 23],
    ['2026-11-01T12:00:00-05:00', '2026-11-01T04:00:00.000Z', '2026-11-02T05:00:00.000Z', 25],
  ])('preserves New York local midnight on DST transition %s', (now, start, end, hours) => {
    const range = statsRange(new Date(now), 1, 'America/New_York')
    expect(range).toEqual({ start, end, timezone: 'America/New_York' })
    const previous = previousRange(range)
    expect(previous.end).toBe(start)
    expect(Date.parse(previous.end) - Date.parse(previous.start)).toBe(Number(hours) * 3600000)
  })
  it('covers seven calendar days across the spring clock change', () => {
    expect(statsRange(new Date('2026-03-10T12:00:00-04:00'), 7, 'America/New_York')).toEqual({ start: '2026-03-04T05:00:00.000Z', end: '2026-03-11T04:00:00.000Z', timezone: 'America/New_York' })
  })
})

describe('overview calculations', () => {
  it('excludes pending orders from success rate and represents an empty denominator', () => {
    expect(successRate(0, 0)).toBeNull()
    expect(successRate(0, 2)).toBe(0)
    expect(successRate(3, 1)).toBe(0.75)
  })
  it('handles no previous receipts, zero current receipts and negative growth', () => {
    expect(amountChange(500, 0)).toBeNull()
    expect(amountChange(0, 1000)).toBe(-1)
    expect(amountChange(750, 1000)).toBe(-0.25)
    expect(amountChange(1250, 1000)).toBe(0.25)
  })
  it('formats cents only at display time, including one cent and English currency', () => {
    expect(money(1, 'zh-CN')).toBe('¥0.01')
    expect(money(123456, 'en')).toBe('¥1,234.56')
  })
})
