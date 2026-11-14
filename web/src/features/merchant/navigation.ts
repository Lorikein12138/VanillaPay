import { Bell, ChartNoAxesCombined, CodeXml, CreditCard, QrCode, ShoppingBag, Smartphone } from 'lucide-react'

// Equal-length labels retain the product priority shown here. Later slices
// enable their item without changing its position or path.
export const merchantNavigation = [
  { label: '概览', key: 'overview', path: '/dashboard', icon: ChartNoAxesCombined, enabled: true },
  { label: '订单', key: 'orders', path: '/orders', icon: ShoppingBag, enabled: true },
  { label: '通知', key: 'notifications', path: '/notifications', icon: Bell, enabled: true },
  { label: '收款码', key: 'codes', path: '/codes', icon: QrCode, enabled: true },
  { label: '监控设备', key: 'devices', path: '/devices', icon: Smartphone, enabled: true },
  { label: '接入配置', key: 'integration', path: '/integration', icon: CodeXml, enabled: true },
  { label: '订阅套餐', key: 'subscription', path: '/subscription', icon: CreditCard, enabled: true },
] as const

export function restrictedMerchantPath(user: { banned: boolean; approval_state?: 'pending' | 'rejected' }) {
  if (user.banned) return '/banned'
  if (user.approval_state) return '/registration-review'
}

export function isMerchantPath(path: string) {
  return path === '/registration-review' || path === '/banned' || path === '/account' || path === '/account/logs' || merchantNavigation.some((item) => path === item.path || path.startsWith(`${item.path}/`))
}
