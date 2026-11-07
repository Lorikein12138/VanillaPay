import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { APIError, consoleApi } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Field } from '@/components/ui/field'
import { FileInput } from '@/components/ui/file-input'
import { Notice } from '@/components/ui/notice'
import { AccountField } from '@/features/auth/account-shared'
import type { SettingsDrafts } from '../settings-data'

type Site = SettingsDrafts['site']

async function readDimensions(file: File, signal: AbortSignal) {
  const url = URL.createObjectURL(file)
  const image = new Image()
  const release = () => {
    image.removeAttribute('src')
    URL.revokeObjectURL(url)
  }
  signal.addEventListener('abort', release, { once: true })
  try {
    image.src = url
    await image.decode()
    return { width: image.naturalWidth, height: image.naturalHeight }
  } finally {
    signal.removeEventListener('abort', release)
    if (!signal.aborted) release()
  }
}

export function SiteForm({
  value,
  update,
  error,
  setUploading,
}: {
  value: Site
  update: (patch: Partial<Site>) => void
  error?: string
  setUploading: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const [uploadError, setUploadError] = useState('')
  const active = useRef<AbortController | null>(null)
  useEffect(
    () => () => {
      active.current?.abort()
      setUploading(false)
    },
    [setUploading],
  )

  async function upload(file?: File) {
    if (!file) return
    active.current?.abort()
    active.current = null
    if (!['image/png', 'image/jpeg'].includes(file.type) || file.size > 5 * 1024 * 1024) {
      setUploadError(t('consoleSettings.logoTypeSize'))
      setUploading(false)
      return
    }
    const controller = new AbortController()
    active.current = controller
    const { signal } = controller
    setUploading(true)
    setUploadError('')
    try {
      const { width, height } = await readDimensions(file, signal)
      if (signal.aborted) return
      if (width > 4096 || height > 4096) {
        setUploadError(t('consoleSettings.logoDimensions'))
        return
      }
      const result = await consoleApi.uploadLogo(file, signal)
      if (!signal.aborted) update({ logo_asset_id: result.asset_id })
    } catch (cause) {
      if (signal.aborted) return
      const code = cause instanceof APIError ? cause.problem?.code : undefined
      setUploadError(
        t(
          code === 'UPLOAD_TOO_LARGE' || code === 'UNSUPPORTED_MEDIA_TYPE'
            ? 'consoleSettings.logoTypeSize'
            : 'consoleSettings.logoUploadFailed',
        ),
      )
    } finally {
      if (!signal.aborted) {
        active.current = null
        setUploading(false)
      }
    }
  }

  return (
    <div className="space-y-5">
      <AccountField
        label={t('consoleSettings.siteName')}
        value={value.site_name}
        maxLength={80}
        error={error}
        onChange={(event) => update({ site_name: event.target.value })}
      />
      <Field label={t('consoleSettings.logo')} htmlFor="logo-file" hint={t('consoleSettings.logoPending')}>
        <FileInput
          id="logo-file"
          accept="image/png,image/jpeg"
          onChange={(event) => void upload(event.target.files?.[0])}
        />
      </Field>
      {uploadError && <Notice tone="error">{uploadError}</Notice>}
      {value.logo_asset_id && (
        <div className="flex items-center gap-4">
          <img
            className="size-16 rounded-control border border-border object-contain"
            src={`/media/${encodeURIComponent(value.logo_asset_id)}`}
            alt={t('consoleSettings.logo')}
          />
          <Button onClick={() => update({ logo_asset_id: null })}>{t('consoleSettings.removeLogo')}</Button>
        </div>
      )}
    </div>
  )
}
