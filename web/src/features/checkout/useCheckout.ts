import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { api, APIError } from '@/api/client'
import { useCountdown } from '@/components/ui/countdown'

export function checkoutDeadline(serverTime: string, expiresAt: string, receivedAt: number) {
  return Date.parse(expiresAt) - (Date.parse(serverTime) - receivedAt)
}

export function useCheckout(tradeNo: string) {
  const [visible, setVisible] = useState(() => document.visibilityState !== 'hidden')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>()
  const mounted = useRef(false)
  const controller = useRef<AbortController | null>(null)
  const query = useQuery({
    queryKey: ['checkout', tradeNo],
    queryFn: async ({ signal }) => {
      const checkout = await api.checkout(tradeNo, signal)
      return { checkout, deadline: checkoutDeadline(checkout.server_time, checkout.expires_at, Date.now()) }
    },
    retry: false, gcTime: 0, refetchOnWindowFocus: false,
    refetchInterval: (current) => visible && current.state.data?.checkout.status === 'pending' && !(current.state.error instanceof APIError && current.state.error.status === 404) ? 3000 : false,
    refetchIntervalInBackground: false,
  })
  const pending = query.data?.checkout.status === 'pending'
  const missing = query.error instanceof APIError && query.error.status === 404
  const refetch = query.refetch
  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false; controller.current?.abort() }
  }, [])
  useEffect(() => {
    const refresh = () => {
      const shown = document.visibilityState !== 'hidden'
      setVisible(shown)
      if (shown && pending && !missing) void refetch({ cancelRefetch: false })
    }
    document.addEventListener('visibilitychange', refresh)
    window.addEventListener('focus', refresh)
    return () => {
      document.removeEventListener('visibilitychange', refresh)
      window.removeEventListener('focus', refresh)
    }
  }, [pending, missing, refetch])
  const seconds = useCountdown(pending ? query.data?.deadline : null)
  const checkedExpiry = useRef<string | undefined>(undefined)
  const expiresAt = query.data?.checkout.expires_at
  useEffect(() => {
    if (pending && !missing && visible && seconds === 0 && expiresAt && checkedExpiry.current !== expiresAt) {
      checkedExpiry.current = expiresAt
      void refetch({ cancelRefetch: false })
    }
  }, [pending, missing, visible, seconds, expiresAt, refetch])

  async function select(channel: string) {
    if (controller.current || !pending || query.data?.checkout.channel) return
    const active = new AbortController()
    controller.current = active
    setBusy(true); setError(undefined)
    try {
      await api.selectCheckoutChannel(tradeNo, { channel }, active.signal)
      // The POST DTO is incomplete. Only a fresh GET can authorize the QR view.
      if (mounted.current) await refetch()
    } catch (failure) {
      if (!mounted.current) return
      setError(failure)
      if (failure instanceof APIError && (['CHANNEL_LOCKED', 'ORDER_NOT_PAYABLE', 'CHANNEL_NOT_ALLOWED', 'QR_UNAVAILABLE'].includes(failure.problem?.code ?? '') || failure.status === 404)) await refetch()
    } finally {
      if (controller.current === active) controller.current = null
      if (mounted.current) setBusy(false)
    }
  }
  return { ...query, seconds, busy, select, actionError: error, missing }
}
