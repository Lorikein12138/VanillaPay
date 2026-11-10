const calendarDate = /^(\d{4})-(\d{2})-(\d{2})$/
const clock = '(?:[01]\\d|2[0-3]):[0-5]\\d'
const localDateTime = new RegExp(`^\\d{4}-\\d{2}-\\d{2}T${clock}(?::[0-5]\\d(?:\\.\\d{1,3})?)?$`)
const rfc3339 = new RegExp(`^\\d{4}-\\d{2}-\\d{2}T${clock}:[0-5]\\d(?:\\.\\d+)?(?:Z|[+-]${clock})$`)

export function parseCalendarDate(value: string) {
  const match = calendarDate.exec(value)
  if (!match || match[0] !== value) return
  const year = Number(match[1]), month = Number(match[2]), day = Number(match[3])
  const date = new Date(0)
  date.setUTCFullYear(year, month - 1, day)
  if (date.getUTCFullYear() !== year || date.getUTCMonth() !== month - 1 || date.getUTCDate() !== day) return
  date.setHours(12, 0, 0, 0)
  date.setFullYear(year, month - 1, day)
  return date
}

export function readRFC3339(value: unknown) {
  if (typeof value === 'string' && rfc3339.exec(value)?.[0] === value && parseCalendarDate(value.slice(0, 10))) return value
}

export function parseLocalDateTime(value: string) {
  if (localDateTime.exec(value)?.[0] !== value || !parseCalendarDate(value.slice(0, 10))) return NaN
  return Date.parse(value)
}

const part = (value: number) => String(value).padStart(2, '0')

export function calendarDateValue(date: Date) {
  return `${String(date.getFullYear()).padStart(4, '0')}-${part(date.getMonth() + 1)}-${part(date.getDate())}`
}

export function localDateInput(value?: string) {
  if (!value || Number.isNaN(Date.parse(value))) return ''
  const date = new Date(value)
  const seconds = date.getSeconds()
  const time = `${part(date.getHours())}:${part(date.getMinutes())}`
  return calendarDateValue(date) + 'T' + time + (seconds ? ':' + part(seconds) : '')
}
