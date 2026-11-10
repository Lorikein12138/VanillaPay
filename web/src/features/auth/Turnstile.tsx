import { useEffect, useRef } from 'react'

type WidgetOptions = {
  sitekey: string
  action: string
  size: 'flexible' | 'compact'
  callback: (token: string) => void
  'expired-callback': () => void
  'error-callback': () => void
}
type TurnstileAPI = { render: (node: HTMLElement, options: WidgetOptions) => string; remove: (id: string) => void }
declare global { interface Window { turnstile?: TurnstileAPI } }

let scriptPromise: Promise<void> | undefined
function loadScript() {
  if (window.turnstile) return Promise.resolve()
  scriptPromise ??= new Promise<void>((resolve, reject) => {
    const script = document.createElement('script')
    script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
    script.async = true
    const fail = () => {
      window.clearTimeout(timeout)
      scriptPromise = undefined
      script.remove()
      reject(new Error('Turnstile unavailable'))
    }
    const timeout = window.setTimeout(fail, 15000)
    script.onload = () => { window.clearTimeout(timeout); if (window.turnstile) resolve(); else fail() }
    script.onerror = fail
    document.head.append(script)
  })
  return scriptPromise
}

export function Turnstile({ siteKey, action, onToken, onError }: {
  siteKey: string; action: string; onToken: (token: string) => void; onError: () => void
}) {
  const container = useRef<HTMLDivElement>(null)
  useEffect(() => {
    let disposed = false
    let id: string | undefined
    let observer: ResizeObserver | undefined
    let size: WidgetOptions['size'] | undefined
    void loadScript().then(() => {
      if (disposed || !container.current || !window.turnstile) return
      let generation = 0
      const render = () => {
        if (!container.current || !window.turnstile) return
        const nextSize = container.current.clientWidth < 300 ? 'compact' : 'flexible'
        if (nextSize === size) return
        const current = ++generation
        if (id) window.turnstile.remove(id)
        size = nextSize
        onToken('')
        id = window.turnstile.render(container.current, {
          sitekey: siteKey, action, size,
          callback: (token) => { if (!disposed && generation === current) onToken(token) },
          'expired-callback': () => { if (!disposed && generation === current) { onToken(''); onError() } },
          'error-callback': () => { if (!disposed && generation === current) { onToken(''); onError() } },
        })
      }
      render()
      observer = new ResizeObserver(render)
      observer.observe(container.current)
    }).catch(() => { if (!disposed) onError() })
    return () => {
      disposed = true
      observer?.disconnect()
      if (id) window.turnstile?.remove(id)
    }
  }, [siteKey, action, onToken, onError])
  return <div ref={container} className="flex min-h-16 w-full justify-center" />
}
