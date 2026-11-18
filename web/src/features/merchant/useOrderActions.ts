import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { api } from '@/api/client'
import type { components } from '@/api/schema'
import { orderDetailQuery } from '@/app/portal-queries'
import { canSupplementOrder, orderErrorKey, type OrderAction } from './orders-data'

type Detail = components['schemas']['OrderDetail']
type OrderNotice =
  | { tone: 'success'; message: `merchant.orders.success.${OrderAction}` }
  | { tone: 'info'; message: 'merchant.orders.errors.supplementUnavailable' }
  | { tone: 'error'; message: ReturnType<typeof orderErrorKey> }

export function invalidateOrderViews(client: QueryClient, name: string, tradeNo: string, detailRefetch: 'active' | 'none' = 'active') {
  return Promise.all([
    client.invalidateQueries({ queryKey: orderDetailQuery('merchant', name, tradeNo).queryKey, exact: true, refetchType: detailRefetch }),
    client.invalidateQueries({ queryKey: ['merchant', name, 'orders'] }),
    client.invalidateQueries({ queryKey: ['merchant', name, 'stats'] }),
  ])
}

export async function loadLatestOrder(client: QueryClient, name: string, tradeNo: string) {
  const detail = await api.order(tradeNo)
  const { queryKey } = orderDetailQuery('merchant', name, tradeNo)
  await client.cancelQueries({ queryKey, exact: true })
  client.setQueryData(queryKey, detail)
  return detail
}

export function useOrderActions(name: string) {
  const client = useQueryClient()
  const [confirmation, setConfirmation] = useState<{ action: OrderAction; key: string; detail: Detail }>()
  const [notice, setNotice] = useState<OrderNotice>()
  const [openingTrade, setOpeningTrade] = useState<string>()
  const opening = useRef(false)

  function open(action: OrderAction, detail: Detail) {
    setNotice(undefined)
    setConfirmation({ action, key: crypto.randomUUID(), detail })
  }

  async function openSupplement(tradeNo: string) {
    if (opening.current || confirmation) return
    opening.current = true
    setOpeningTrade(tradeNo)
    setNotice(undefined)
    try {
      const detail = await loadLatestOrder(client, name, tradeNo)
      if (canSupplementOrder(detail.order)) {
        open('supplement', detail)
      } else {
        setNotice({ tone: 'info', message: 'merchant.orders.errors.supplementUnavailable' })
        await invalidateOrderViews(client, name, tradeNo, 'none')
      }
    } catch (error) {
      setNotice({ tone: 'error', message: orderErrorKey(error) })
    } finally {
      opening.current = false
      setOpeningTrade(undefined)
    }
  }

  async function completed() {
    if (!confirmation) return
    await invalidateOrderViews(client, name, confirmation.detail.order.trade_no)
    setNotice({ tone: 'success', message: `merchant.orders.success.${confirmation.action}` })
    setConfirmation(undefined)
  }

  return {
    confirmation, notice, openingTrade, open, openSupplement, completed,
    close: () => setConfirmation(undefined),
    dismissNotice: () => setNotice(undefined),
  }
}
