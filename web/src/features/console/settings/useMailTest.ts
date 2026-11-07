import { useEffect, useRef, useState } from 'react'
import { consoleApi } from '@/api/client'

type State = 'idle' | 'queued' | 'sent' | 'failed' | 'timeout'
type Delivery = { controller: AbortController; deadline: ReturnType<typeof setTimeout> }

function waitForPoll(signal: AbortSignal) {
  return new Promise<void>((resolve) => {
    function finish() {
      clearTimeout(timer)
      signal.removeEventListener('abort', finish)
      resolve()
    }
    const timer = setTimeout(finish, 2000)
    signal.addEventListener('abort', finish, { once: true })
  })
}

function stop(delivery: Delivery) {
  clearTimeout(delivery.deadline)
  delivery.controller.abort()
}

export function useMailTest() {
  const [state, setState] = useState<State>('idle')
  const active = useRef<Delivery | null>(null)
  useEffect(
    () => () => {
      if (active.current) {
        stop(active.current)
        active.current = null
      }
    },
    [],
  )

  async function send(recipient: string) {
    if (active.current) return
    const controller = new AbortController()
    const { signal } = controller
    function finish(next: State) {
      if (active.current !== delivery) return
      stop(delivery)
      active.current = null
      setState(next)
    }
    const delivery = { controller, deadline: setTimeout(() => finish('timeout'), 60000) }
    active.current = delivery
    setState('queued')
    try {
      const queued = await consoleApi.testMail(recipient, signal)
      while (!signal.aborted) {
        await waitForPoll(signal)
        if (signal.aborted) return
        const status = await consoleApi.mailTestStatus(queued.delivery_id, signal)
        if (status.state === 'sent' || status.state === 'failed') {
          finish(status.state)
          return
        }
      }
    } catch {
      finish('failed')
    }
  }

  return { state, send }
}
