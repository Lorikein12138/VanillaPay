import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, APIError } from '@/api/client'
import { buttonVariants } from '@/components/ui/button'
import { ArrowDown, ArrowRight, ChartColumn, Check, CodeXml, CreditCard, ShieldCheck, Smartphone, Target, type LucideIcon } from 'lucide-react'
import { Fragment, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { PublicFooter } from '@/components/PublicFooter'
import { useSiteBrand } from '@/components/SiteBrand'
import { TopBar } from '@/components/TopBar'
import { parsePublicURL, usePublicConfig } from '@/lib/public-config'
import { cn } from '@/lib/utils'
import { money } from '@/features/merchant/overview-data'

const featureIcons = {
  matching: Target,
  compat: CodeXml,
  device: Smartphone,
  stats: ChartColumn,
  security: ShieldCheck,
  subscription: CreditCard,
} as const satisfies Record<string, LucideIcon>

const stepKeys = ['register', 'configure', 'integrate'] as const

const navLinkClass =
  'hidden h-10 items-center rounded-control px-3 text-label text-text-2 transition-colors hover:bg-hover hover:text-text md:inline-flex'

function homeDestination(target: string | null | undefined) {
  const destination = parsePublicURL(target, location.href)
  if (destination?.protocol !== 'https:') return undefined
  const path = destination.pathname.replace(/\/+$/, '')
  if (destination.origin === location.origin && (path === '' || path === '/index.html')) return undefined
  return destination.href
}

export function HomePage() {
  const { t } = useTranslation()
  const config = usePublicConfig()
  const { name } = useSiteBrand()
  const home = config.isError ? undefined : config.data?.home
  const destination = home?.mode === 'url' ? homeDestination(home.url) : undefined

  if (config.isPending) {
    return (
      <main id="main" className="grid min-h-dvh place-items-center px-4 text-label text-text-2">
        <p role="status">{t('polish.loading')}</p>
      </main>
    )
  }
  if (home?.mode === 'html' && home.html != null) {
    return (
      <div className="flex h-dvh flex-col">
        <TopBar compact actions={<HomeAccess registrationOpen={config.data?.registration_open} />} />
        <main id="main" className="min-h-0 flex-1">
          <iframe
            title={name}
            className="block h-full w-full border-0"
            srcDoc={home.html}
            sandbox="allow-scripts allow-forms allow-popups allow-popups-to-escape-sandbox allow-top-navigation-by-user-activation"
          />
        </main>
      </div>
    )
  }
  if (destination) return <HomeRedirect url={destination} />
  return <DefaultHome registrationOpen={!config.isError && config.data?.registration_open} />
}

function HomeAccess({ registrationOpen }: { registrationOpen?: boolean }) {
  const { t } = useTranslation()
  const session = useQuery({
    queryKey: ['home-merchant-session'],
    gcTime: 0,
    queryFn: async ({ signal }) => {
      try { return (await api.peekSession(signal)).user.banned ? 'banned' : 'active' }
      catch (error) {
        if (error instanceof APIError && (error.status === 401 || error.status === 403)) return null
        throw error
      }
    },
  })
  if (session.isPending) return null
  const secondary = cn(buttonVariants({ variant: 'secondary', size: 'sm' }), 'px-2 sm:px-3')
  const ghost = cn(buttonVariants({ variant: 'ghost', size: 'sm' }), 'px-2 sm:px-3')
  if (session.data) return (
    <Link to={session.data === 'banned' ? '/banned' : '/dashboard'} className={secondary}>
      {t('home.dashboard')}
      <ArrowRight className="hidden sm:block" strokeWidth={1.6} aria-hidden="true" />
    </Link>
  )
  return <>
    <Link to="/login" className={ghost}>{t('auth.submit')}</Link>
    {registrationOpen && <Link to="/register" className={secondary}>{t('home.register')}</Link>}
  </>
}

function HomeRedirect({ url }: { url: string }) {
  const { t } = useTranslation()
  const lastDestination = useRef<string>(null)
  useEffect(() => {
    if (lastDestination.current === url) return
    lastDestination.current = url
    try {
      location.replace(url)
    } catch {
      // The manual link remains usable if the browser blocks programmatic navigation.
    }
  }, [url])
  return (
    <main id="main" className="grid min-h-dvh place-content-center gap-2 px-4 text-center">
      <p role="status" className="m-0 text-label text-text-2">{t('home.redirecting')}</p>
      <a href={url} className={buttonVariants({ variant: 'link' })}>{t('home.continueToDestination')}</a>
    </main>
  )
}

function DefaultHome({ registrationOpen }: { registrationOpen?: boolean }) {
  const { t, i18n } = useTranslation()
  const sampleTime = '2026-06-18T06:32:00Z'
  const paidAt = new Intl.DateTimeFormat(i18n.language, {
    month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZone: 'Asia/Shanghai',
  }).format(new Date(sampleTime))

  return (
    <>
      <TopBar actions={<HomeAccess registrationOpen={registrationOpen} />}>
        <nav aria-label={t('home.nav.label')} className="contents">
          <a href="#features" className={navLinkClass}>
            {t('home.nav.features')}
          </a>
          <a href="#steps" className={navLinkClass}>
            {t('home.nav.steps')}
          </a>
        </nav>
      </TopBar>

      <main id="main">
        <section className="mx-auto grid max-w-[1200px] items-center gap-8 px-5 py-10 sm:px-6 md:grid-cols-[minmax(0,1.15fr)_minmax(0,0.85fr)] md:gap-10 md:py-20 lg:gap-16">
          <div className="flex min-w-0 flex-col gap-5">
            <h1 className="m-0 font-serif text-[clamp(2.375rem,5.4vw,4rem)] leading-[1.1] font-normal tracking-[-0.02em] text-balance [word-break:keep-all] [overflow-wrap:anywhere]">
              {/* keep-all + <wbr> breaks CJK titles at the comma, not mid-phrase, without changing the accessible name. */}
              {(t('home.title').match(/[^，,]+[，,]?/g) ?? []).map((part, index) => <Fragment key={index}>{index > 0 && <wbr />}{part}</Fragment>)}
            </h1>
            <p className="m-0 max-w-[520px] text-body leading-relaxed text-text-2 sm:text-lg">{t('home.subtitle')}</p>
            <div className="flex flex-wrap gap-2.5">
              <a
                href="#features"
                className={buttonVariants({ variant: 'primary' })}
              >
                {t('home.cta')}
                <ArrowDown className="size-4" strokeWidth={1.8} aria-hidden="true" />
              </a>
              <a href="#steps" className={buttonVariants({ variant: 'secondary' })}>{t('home.nav.steps')}</a>
            </div>
          </div>

          <div className="relative min-w-0 px-2 py-5 before:absolute before:inset-y-0 before:right-0 before:left-[18%] before:rounded-t-[48%] before:rounded-b-control before:bg-accent-soft md:py-9">
            <figure className="receipt-paper mx-auto flex w-full max-w-[390px] rotate-[2deg] flex-col gap-3 px-5 pt-5 pb-5 sm:px-7">
              <figcaption className="flex flex-wrap items-center justify-between gap-2 border-b border-receipt-rule pb-3">
                <strong>{t('home.sampleReceipt')}</strong>
                <span className="rounded-full bg-notice-success px-2.5 py-1 text-caption font-semibold text-dot-ok">
                  {t('checkout.status.paid')}
                </span>
              </figcaption>
              <p className="font-serif text-[3rem] leading-none tracking-tight lining-nums tabular-nums">{money(12800, i18n.language)}</p>
              <p className="text-caption text-text-2">{t('checkout.actual')}</p>
              <dl className="text-label [&>div]:flex [&>div]:flex-wrap [&>div]:items-baseline [&>div]:justify-between [&>div]:gap-x-3 [&>div]:border-t [&>div]:border-dashed [&>div]:border-receipt-rule [&>div]:py-2 [&_dt]:text-caption [&_dt]:text-text-2">
                <div><dt>{t('manage.merchant')}</dt><dd>{t('home.sampleMerchant')}</dd></div>
                <div>
                  <dt>{t('checkout.orderLabel')}</dt>
                  <dd className="font-mono" translate="no">VP-DEMO-240618</dd>
                </div>
                <div><dt>{t('merchant.orders.paidAt')}</dt><dd><time dateTime={sampleTime}>{paidAt}</time></dd></div>
              </dl>
              <p className="flex items-center gap-2 text-label text-text-2">
                <Check aria-hidden="true" className="size-4 text-dot-ok" />
                {t('home.sampleTransaction')}
              </p>
            </figure>
          </div>
        </section>

        <section id="features" className="scroll-mt-4 border-y border-border">
          <div className="mx-auto flex max-w-[1200px] flex-col gap-8 px-5 py-12 sm:px-6 lg:py-16">
            <div className="flex max-w-[640px] flex-col gap-3">
              <h2 className="m-0 font-serif text-[clamp(1.875rem,3.6vw,2.75rem)] leading-[1.15] font-normal tracking-[-0.015em]">
                {t('home.features.title')}
              </h2>
              <p className="m-0 text-[17px] leading-[1.7] text-text-2">{t('home.features.subtitle')}</p>
            </div>
            <ul className="m-0 grid list-none grid-cols-1 gap-x-8 gap-y-6 p-0 md:grid-cols-2 lg:grid-cols-3">
              {(Object.keys(featureIcons) as (keyof typeof featureIcons)[]).map((key) => {
                const Icon = featureIcons[key]
                return (
                  <li key={key} className="flex min-w-0 flex-col gap-2.5 border-t border-border pt-5">
                    <span className="text-accent-text">
                      <Icon className="size-6" strokeWidth={1.5} aria-hidden="true" />
                    </span>
                    <h3 className="m-0 font-serif text-heading font-normal">{t(`home.features.items.${key}.title`)}</h3>
                    <p className="m-0 text-[15px] leading-[1.7] text-text-2">{t(`home.features.items.${key}.body`)}</p>
                  </li>
                )
              })}
            </ul>
          </div>
        </section>

        <section id="steps" className="mx-auto flex max-w-[1200px] scroll-mt-4 flex-col gap-8 px-5 py-12 sm:px-6 lg:py-16">
          <h2 className="m-0 font-serif text-[clamp(1.875rem,3.6vw,2.75rem)] leading-[1.15] font-normal tracking-[-0.015em]">
            {t('home.steps.title')}
          </h2>
          <ol className="m-0 grid list-none grid-cols-1 gap-8 p-0 md:grid-cols-3">
            {stepKeys.map((key, index) => (
              <li key={key} className="flex min-w-0 flex-col gap-3 border-t-[0.5px] border-border-strong pt-6">
                <span className="font-serif text-xl text-accent-text tabular-nums">{String(index + 1).padStart(2, '0')}</span>
                <h3 className="m-0 font-serif text-[1.375rem] leading-[1.3] font-normal">{t(`home.steps.items.${key}.title`)}</h3>
                <p className="m-0 text-[15px] leading-[1.7] text-text-2">{t(`home.steps.items.${key}.body`)}</p>
              </li>
            ))}
          </ol>
        </section>
      </main>

      <PublicFooter />
    </>
  )
}
