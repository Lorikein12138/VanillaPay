import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LogoMark } from '@/components/Logo'
import { usePublicConfig } from '@/lib/public-config'
import { cn } from '@/lib/utils'

export function useSiteBrand() {
  const { t } = useTranslation()
  const { data } = usePublicConfig()
  return { name: data?.site_name.trim() || t('common.brand'), logoUrl: data?.logo_url }
}

export function SiteTitle() {
  const { name } = useSiteBrand()
  useEffect(() => {
    const previousTitle = document.title
    document.title = name
    return () => { document.title = previousTitle }
  }, [name])
  return null
}

export function SiteIcons() {
  const { data } = usePublicConfig()
  const favicon = data?.favicon_url
  useEffect(() => {
    if (!favicon?.startsWith('/favicon.ico?')) return
    const query = new URL(favicon, window.location.origin).search
    document.getElementById('site-icon')?.setAttribute('href', favicon)
    document.getElementById('apple-icon')?.setAttribute('href', '/apple-touch-icon.png' + query)
  }, [favicon])
  return null
}

function SiteLogo({ url, name, className }: { url: string; name: string; className: string }) {
  const [failed, setFailed] = useState(false)
  if (!url || failed) return <LogoMark className={className} />
  return <img src={url} alt={name} className={cn('shrink-0 object-contain', className)} onError={() => setFailed(true)} />
}

export function SiteBrand({ compact = false, labelClassName }: { compact?: boolean; labelClassName?: string }) {
  const { name, logoUrl = '' } = useSiteBrand()
  return (
    <>
      <SiteLogo key={logoUrl} url={logoUrl} name={name} className={compact ? 'size-5' : 'size-7'} />
      <span className={cn(compact ? 'min-w-0 break-words' : 'min-w-0 truncate font-serif text-xl font-[500] tracking-[-0.01em]', labelClassName)}>{name}</span>
    </>
  )
}
