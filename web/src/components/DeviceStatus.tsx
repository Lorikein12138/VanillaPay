import { useTranslation } from 'react-i18next'
import type { components } from '@/api/schema'
import { StatusDot } from './StatusDot'

const tones = { unbound: 'off', offline: 'off', abnormal: 'wait', online: 'ok' } as const
const labels = { unbound: 'manage.unbound', offline: 'manage.offline', abnormal: 'polish.deviceAbnormal', online: 'manage.online' } as const
const bindingLabels = { unbound: 'polish.notBound', offline: 'polish.offlineBound', abnormal: 'polish.deviceAbnormal', online: 'polish.onlineBound' } as const

export function DeviceStatus({ status, showBinding = false }: { status: components['schemas']['DeviceDeviceDTO']['health_status']; showBinding?: boolean }) {
  const { t } = useTranslation()
  return <StatusDot tone={tones[status]}>{t((showBinding ? bindingLabels : labels)[status])}</StatusDot>
}
