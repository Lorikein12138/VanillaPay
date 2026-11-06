import { Bell, ChartNoAxesCombined, ClipboardList, CreditCard, Layers, Settings, ShoppingBag, Store } from 'lucide-react'

export const consoleNavigation = [
  { key: 'overview', path: '/console', icon: ChartNoAxesCombined, enabled: true },
  { key: 'merchants', path: '/console/merchants', icon: Store, enabled: false },
  { key: 'orders', path: '/console/orders', icon: ShoppingBag, enabled: false },
  { key: 'plans', path: '/console/plans', icon: Layers, enabled: false },
  { key: 'announcements', path: '/console/announcements', icon: Bell, enabled: false },
  { key: 'purchases', path: '/console/purchases', icon: CreditCard, enabled: false },
  { key: 'settings', path: '/console/settings', icon: Settings, enabled: false },
  { key: 'auditLogs', path: '/console/audit-logs', icon: ClipboardList, enabled: false },
] as const
