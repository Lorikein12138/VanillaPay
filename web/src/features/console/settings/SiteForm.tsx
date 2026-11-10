import { useCallback, useEffect, useRef, useState } from 'react'
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

export function SiteForm({ value, saved, update, error, setUploading }: {
  value: Site
  saved: Site
  update: (patch: Partial<Site>) => void
  error?: string
  setUploading: (busy: boolean) => void
}) {
  const { t } = useTranslation()
  const uploads = useRef(new Set<ImageKind>())
  const markUpload = useCallback((kind: ImageKind, busy: boolean) => {
    if (busy) uploads.current.add(kind)
    else uploads.current.delete(kind)
    setUploading(uploads.current.size > 0)
  }, [setUploading])
  return <div className="space-y-5">
    <AccountField label={t('consoleSettings.siteName')} value={value.site_name} maxLength={80} error={error}
      onChange={(event) => update({ site_name: event.target.value })} />
    {(['logo', 'favicon'] as const).map((kind) => {
      const key = kind === 'logo' ? 'logo_asset_id' : 'favicon_asset_id'
      return <SiteImageField key={kind} kind={kind} asset={value[key]} saved={saved[key]}
        update={(id) => update({ [key]: id })} markUpload={markUpload} />
    })}
  </div>
}

type ImageKind = 'logo' | 'favicon'

function SiteImageField({ kind, asset, saved, update, markUpload }: {
  kind: ImageKind
  asset: string | null
  saved: string | null
  update: (asset: string | null) => void
  markUpload: (kind: ImageKind, busy: boolean) => void
}) {
  const { t } = useTranslation()
  const favicon = kind === 'favicon'
  const fieldID = `${kind}-file`
  const [uploadError, setUploadError] = useState('')
  const active = useRef<AbortController | null>(null)
  useEffect(
    () => () => {
      active.current?.abort()
      markUpload(kind, false)
    },
    [kind, markUpload],
  )

  async function upload(file?: File) {
    if (!file) return
    active.current?.abort()
    active.current = null
    if (!['image/png', 'image/jpeg'].includes(file.type) || file.size > 5 * 1024 * 1024) {
      setUploadError(t('consoleSettings.logoTypeSize'))
      markUpload(kind, false)
      return
    }
    const controller = new AbortController()
    active.current = controller
    const { signal } = controller
    markUpload(kind, true)
    setUploadError('')
    try {
      const { width, height } = await readDimensions(file, signal)
      if (signal.aborted) return
      if (width > 4096 || height > 4096) {
        setUploadError(t('consoleSettings.logoDimensions'))
        return
      }
      const result = await consoleApi.uploadSiteImage(kind, file, signal)
      if (!signal.aborted) update(result.asset_id)
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
        markUpload(kind, false)
      }
    }
  }

  function changeAsset(id: string | null) {
    active.current?.abort()
    active.current = null
    markUpload(kind, false)
    setUploadError('')
    update(id)
  }

  return (
    <div className="space-y-5">
      <Field label={t(favicon ? 'consoleSettings.favicon' : 'consoleSettings.logo')} htmlFor={fieldID} hint={t(favicon ? 'consoleSettings.faviconPending' : 'consoleSettings.logoPending')}>
        <FileInput
          id={fieldID}
          accept="image/png,image/jpeg"
          onChange={(event) => void upload(event.target.files?.[0])}
        />
      </Field>
      {uploadError && <Notice tone="error">{uploadError}</Notice>}
      {asset && (
        <div className="flex items-center gap-4">
          <img
            className="size-16 rounded-control border border-border object-contain"
            src={`/media/${encodeURIComponent(asset)}`}
            alt={t(favicon ? 'consoleSettings.favicon' : 'consoleSettings.logo')}
          />
          <Button onClick={() => changeAsset(null)}>{t(favicon ? 'consoleSettings.removeFavicon' : 'consoleSettings.removeLogo')}</Button>
        </div>
      )}
      {asset !== saved && <Button variant="ghost" onClick={() => changeAsset(saved)}>{t(favicon ? 'consoleSettings.undoFavicon' : 'consoleSettings.undoLogo')}</Button>}
    </div>
  )
}
