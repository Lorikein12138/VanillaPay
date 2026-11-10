import { Select } from '@/components/ui/select'
import { useId, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { StatusDot } from '@/components/StatusDot'
import { Field } from '@/components/ui/field'
import { Checkbox, Textarea } from '@/components/ui/input'
import { ChannelList, sortChannels } from '@/components/ui/payment'
import type { Schema } from './manage-data'

export function Choice({ label, value, onChange, children, disabled = false }: {
  label: string; value: string; onChange: (value: string) => void; children: ReactNode; disabled?: boolean
}) {
  const id = useId()
  return <Field htmlFor={id} label={label}>
    <Select id={id} value={value} disabled={disabled} onValueChange={onChange}>{children}</Select>
  </Field>
}
export function Reason({ value, onChange, hint, disabled }: { value: string; onChange: (value: string) => void; hint: string; disabled: boolean }) {
  const id = useId(), { t } = useTranslation()
  return <Field htmlFor={id} label={t('manage.reason')} hint={hint}>
    <Textarea id={id} value={value} disabled={disabled} aria-describedby={id + '-hint'} onChange={(event) => onChange(event.target.value)} />
  </Field>
}
export function ChannelChoices({ allowed, value, onChange, disabled }: {
  allowed: readonly string[]; value: string[]; onChange: (value: string[]) => void; disabled: boolean
}) {
  const { t } = useTranslation()
  return <fieldset disabled={disabled} className="m-0 min-w-0 border-0 p-0"><legend className="mb-1.5 p-0 text-label text-text-2">{t('manage.channels')}</legend><div className="flex flex-wrap items-center gap-x-5 gap-y-2">
    {sortChannels(allowed).map((channel) => <Checkbox key={channel} checked={value.includes(channel)}
      onChange={(event) => onChange(event.target.checked ? [...value, channel] : value.filter((item) => item !== channel))}>
      {t(`merchant.overview.channels.${channel}`, { defaultValue: channel })}
    </Checkbox>)}
  </div></fieldset>
}
export function Channels({ value }: { value: string[] | null }) {
  const { t } = useTranslation()
  return value?.length
    ? <ChannelList channels={value} name={(channel) => t(`merchant.overview.channels.${channel}`, { defaultValue: channel })} />
    : null
}
export function MerchantStatus({ banned }: { banned: boolean }) {
  const { t } = useTranslation()
  return <StatusDot tone={banned ? 'off' : 'ok'}>{t(banned ? 'manage.banned' : 'manage.normal')}</StatusDot>
}
export function DeviceStatus({ device }: { device: Schema['DeviceDeviceDTO'] }) {
  const { t } = useTranslation()
  return <StatusDot tone={!device.bound ? 'off' : device.online ? 'ok' : 'wait'}>
    {t(!device.bound ? 'manage.unbound' : device.online ? 'manage.online' : 'manage.offline')}
  </StatusDot>
}
