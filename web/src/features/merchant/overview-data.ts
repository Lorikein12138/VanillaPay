export type StatsRange = { start: string; end: string; timezone: string }
export type OverviewDays = 1 | 7 | 30

// Keep calendar dates separate from instants: a local day can be 23 or 25 hours.
export function statsRange(now: Date, days: OverviewDays, timezone: string): StatsRange {
  const formatter = new Intl.DateTimeFormat('en-US', { timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' })
  const parts = (date: Date) => {
    const values = formatter.formatToParts(date)
    const read = (type: Intl.DateTimeFormatPartTypes) => Number(values.find((part) => part.type === type)?.value)
    return { year: read('year'), month: read('month'), day: read('day'), hour: read('hour'), minute: read('minute'), second: read('second') }
  }
  const today = parts(now)
  const calendar = Date.UTC(today.year, today.month - 1, today.day)
  function midnight(day: number) {
    let candidate = day
    let previous = NaN
    for (let i = 0; i < 4; i++) {
      const p = parts(new Date(candidate))
      const offset = Date.UTC(p.year, p.month - 1, p.day, p.hour, p.minute, p.second) - candidate
      const next = day - offset
      if (next === candidate) return new Date(candidate).toISOString()
      // If midnight is skipped, choose the instant after the clock gap.
      if (next === previous) return new Date(Math.max(candidate, next)).toISOString()
      previous = candidate
      candidate = next
    }
    return new Date(candidate).toISOString()
  }
  return { start: midnight(calendar - (days - 1) * 86400000), end: midnight(calendar + 86400000), timezone }
}

export function previousRange(range: StatsRange): StatsRange {
  const start = Date.parse(range.start)
  return { start: new Date(start - (Date.parse(range.end) - start)).toISOString(), end: range.start, timezone: range.timezone }
}

export function successRate(paid: number, expired: number): number | null {
  return paid + expired === 0 ? null : paid / (paid + expired)
}

export function amountChange(currentCents: number, previousCents: number): number | null {
  return previousCents === 0 ? null : (currentCents - previousCents) / previousCents
}

export function money(cents: number, language: string) {
  return new Intl.NumberFormat(language, { style: 'currency', currency: 'CNY', currencyDisplay: 'narrowSymbol' }).format(cents / 100)
}

export function relativeTime(timestamp: string, now: Date, language: string) {
  const seconds = Math.min(0, Math.round((Date.parse(timestamp) - now.getTime()) / 1000))
  const formatter = new Intl.RelativeTimeFormat(language, { numeric: 'auto' })
  for (const [unit, size] of [['day', 86400], ['hour', 3600], ['minute', 60], ['second', 1]] as const) {
    if (Math.abs(seconds) >= size || unit === 'second') return formatter.format(Math.round(seconds / size), unit)
  }
}
