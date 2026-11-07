import { Bell, ChartNoAxesCombined, ClipboardList, CreditCard, Layers, Settings, ShoppingBag, Store } from 'lucide-react'

export const consoleNavigation = [
  { key: 'overview', path: '/console', icon: ChartNoAxesCombined, enabled: true },
  { key: 'merchants', path: '/console/merchants', icon: Store, enabled: true },
  { key: 'orders', path: '/console/orders', icon: ShoppingBag, enabled: true },
  { key: 'plans', path: '/console/plans', icon: Layers, enabled: true },
  { key: 'announcements', path: '/console/announcements', icon: Bell, enabled: false },
  { key: 'purchases', path: '/console/purchases', icon: CreditCard, enabled: true },
  { key: 'settings', path: '/console/settings', icon: Settings, enabled: false },
  { key: 'auditLogs', path: '/console/audit-logs', icon: ClipboardList, enabled: false },
] as const
