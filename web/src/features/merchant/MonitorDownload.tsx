import { useQuery } from '@tanstack/react-query'
import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button, buttonVariants } from '@/components/ui/button'
import { CopyField } from '@/components/ui/copy-button'
import { Card, SectionHeader } from '@/components/ui/layout'
import { Notice } from '@/components/ui/notice'

const versionPattern = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/
const timestampPattern = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/

export function parseMonitorManifest(value: unknown) {
  if (typeof value !== 'object' || value === null) return null
  const { version, file, sha256, size, published_at } = value as Record<string, unknown>
  if (Object.keys(value).length !== 5 || typeof version !== 'string' || !versionPattern.test(version) ||
    file !== `vanillapay-monitor-${version}.apk` || typeof sha256 !== 'string' || !/^[a-f0-9]{64}$/i.test(sha256) ||
    typeof size !== 'number' || !Number.isSafeInteger(size) || size <= 0 ||
    typeof published_at !== 'string' || !timestampPattern.test(published_at) || !Number.isFinite(Date.parse(published_at))) return null
  return { version, file, sha256, size }
}

async function getMonitorManifest({ signal }: { signal: AbortSignal }) {
  const response = await fetch(new URL('/downloads/latest.json', location.origin), { signal, redirect: 'error', credentials: 'omit' })
  return response.ok ? parseMonitorManifest(await response.json() as unknown) : null
}

export function MonitorDownload() {
  const { t, i18n } = useTranslation()
  const checksumId = useId()
  const [showChecksum, setShowChecksum] = useState(false)
  const manifest = useQuery({ queryKey: ['monitor-download'], queryFn: getMonitorManifest, staleTime: 60_000, retry: false })
  const data = manifest.isError ? null : manifest.data
  const format = new Intl.NumberFormat(i18n.language, { style: 'unit', unit: 'byte', unitDisplay: 'short' })
  return (
    <Card aria-busy={manifest.isPending}>
      <SectionHeader title={t('monitorDownload.title')} />
      <p className="mb-3 text-text-2">{t('monitorDownload.instructions')}</p>
      {manifest.isPending ? <p className="text-text-3">{t('monitorDownload.loading')}</p> : data ? (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-3">
            <a className={buttonVariants({ variant: 'secondary' })} href={`/downloads/${data.file}`} download>
              {t('monitorDownload.download', { version: data.version })}
            </a>
            <span className="text-caption text-text-3">{format.format(data.size)}</span>
          </div>
          <Button variant="link" size="sm" aria-expanded={showChecksum} aria-controls={checksumId}
            onClick={() => setShowChecksum(!showChecksum)}>
            {t('monitorDownload.checksum')}
          </Button>
          <div id={checksumId} hidden={!showChecksum}>
            <CopyField className="mt-2" value={data.sha256} label={t('monitorDownload.copyChecksum')} />
          </div>
        </div>
      ) : <Notice tone="info">{t('monitorDownload.unavailable')}</Notice>}
    </Card>
  )
}
