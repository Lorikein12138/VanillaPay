import { useEffect, useState } from 'react'

/** 0:59 / 4:05 / 12:00 — never negative. */
export function formatCountdown(seconds: number) {
  const total = Math.max(0, Math.ceil(seconds))
  return `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`
}

/** Seconds left until `deadline` (ISO string or epoch ms), ticking once a second. */
export function useCountdown(deadline: string | number | null | undefined) {
  const target = deadline == null ? Number.NaN : typeof deadline === 'number' ? deadline : Date.parse(deadline)
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (Number.isNaN(target)) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [target])
  return Number.isNaN(target) ? 0 : Math.max(0, Math.ceil((target - now) / 1000))
}
