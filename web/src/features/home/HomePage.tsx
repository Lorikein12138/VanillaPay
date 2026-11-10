import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { api, APIError } from '@/api/client'
import { buttonVariants } from '@/components/ui/button'
import { ArrowDown, ArrowRight, BellRing, ChartColumn, CodeXml, CreditCard, QrCode, ShieldCheck, ShoppingCart, Smartphone, Target, type LucideIcon } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { PublicFooter } from '@/components/PublicFooter'
import { useSiteBrand } from '@/components/SiteBrand'
import { TopBar } from '@/components/TopBar'
import { parsePublicURL, usePublicConfig } from '@/lib/public-config'
import { cn } from '@/lib/utils'

const featureIcons = {
  matching: Target,
  compat: CodeXml,
  device: Smartphone,
  stats: ChartColumn,
  security: ShieldCheck,
  subscription: CreditCard,
} as const satisfies Record<string, LucideIcon>

const flowIcons = {
  create: ShoppingCart,
  pay: QrCode,
  receive: Smartphone,
  notify: BellRing,
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
  const secondary = cn(buttonVariants({ variant: 'secondary', size: 'sm' }), 'h-9 px-2 sm:px-3')
  const ghost = cn(buttonVariants({ variant: 'ghost', size: 'sm' }), 'h-9 px-2 sm:px-3')
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
  const { t } = useTranslation()

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
        <section className="mx-auto flex max-w-[1200px] flex-wrap items-center gap-12 px-4 pt-16 pb-20 sm:px-6 lg:gap-16 lg:pt-24 lg:pb-22">
          <div className="flex min-w-0 flex-[1_1_460px] flex-col gap-7">
            <span className="text-label font-medium text-accent-text">{t('home.eyebrow')}</span>
            <h1 className="m-0 font-serif text-[clamp(2.25rem,5.4vw,4rem)] leading-[1.12] font-[330] text-balance">
              {t('home.title')}
            </h1>
            <p className="m-0 max-w-[520px] text-lg leading-[1.7] text-text-2">{t('home.subtitle')}</p>
            <div>
              <a
                href="#features"
                className={buttonVariants({ variant: 'primary' })}
              >
                {t('home.cta')}
                <ArrowDown className="size-4" strokeWidth={1.8} aria-hidden="true" />
              </a>
            </div>
          </div>

          <div className="flex min-w-0 flex-[1_1_420px] justify-center">
            <figure className="m-0 flex w-full max-w-[440px] flex-col gap-5 rounded-dialog border-[0.5px] border-border bg-surface p-6 shadow-[0_1px_2px_rgba(20,20,19,0.04),0_12px_32px_rgba(20,20,19,0.06)] sm:p-7">
              <figcaption className="text-label text-text-2">{t('home.flowCard.title')}</figcaption>
              <ol className="m-0 flex list-none flex-col p-0">
                {(Object.keys(flowIcons) as (keyof typeof flowIcons)[]).map((key, index, all) => {
                  const Icon = flowIcons[key]
                  const last = index === all.length - 1
                  return (
                    <li key={key} className="flex gap-4">
                      <div className="flex flex-col items-center">
                        <span className="grid size-10 shrink-0 place-items-center rounded-full bg-muted text-accent-text">
                          <Icon className="size-[18px]" strokeWidth={1.7} aria-hidden="true" />
                        </span>
                        {last ? null : <span className="my-1 w-px flex-1 bg-border-strong" aria-hidden="true" />}
                      </div>
                      <p className={last ? 'm-0 pt-2' : 'm-0 pt-2 pb-6'}>{t(`home.flowCard.steps.${key}`)}</p>
                    </li>
                  )
                })}
              </ol>
            </figure>
          </div>
        </section>

        <section id="features" className="scroll-mt-4 border-y-[0.5px] border-border bg-surface">
          <div className="mx-auto flex max-w-[1200px] flex-col gap-12 px-4 py-20 sm:px-6 lg:py-24">
            <div className="flex max-w-[640px] flex-col gap-3">
              <h2 className="m-0 font-serif text-[clamp(1.875rem,3.5vw,2.5rem)] leading-[1.2] font-normal">
                {t('home.features.title')}
              </h2>
              <p className="m-0 text-[17px] leading-[1.7] text-text-2">{t('home.features.subtitle')}</p>
            </div>
            <ul className="m-0 grid list-none grid-cols-1 gap-px overflow-hidden rounded-card border-[0.5px] border-border bg-border p-0 md:grid-cols-2 lg:grid-cols-3">
              {(Object.keys(featureIcons) as (keyof typeof featureIcons)[]).map((key) => {
                const Icon = featureIcons[key]
                return (
                  <li key={key} className="flex min-w-0 flex-col gap-3 bg-surface p-6 sm:p-8">
                    <span className="grid size-10 place-items-center rounded-[10px] bg-muted text-accent-text">
                      <Icon className="size-5" strokeWidth={1.7} aria-hidden="true" />
                    </span>
                    <h3 className="m-0 font-serif text-heading font-medium">{t(`home.features.items.${key}.title`)}</h3>
                    <p className="m-0 text-[15px] leading-[1.7] text-text-2">{t(`home.features.items.${key}.body`)}</p>
                  </li>
                )
              })}
            </ul>
          </div>
        </section>

        <section id="steps" className="mx-auto flex max-w-[1200px] scroll-mt-4 flex-col gap-12 px-4 py-20 sm:px-6 lg:py-24">
          <h2 className="m-0 font-serif text-[clamp(1.875rem,3.5vw,2.5rem)] leading-[1.2] font-normal">
            {t('home.steps.title')}
          </h2>
          <ol className="m-0 grid list-none grid-cols-1 gap-6 p-0 md:grid-cols-3">
            {stepKeys.map((key, index) => (
              <li key={key} className="flex min-w-0 flex-col gap-3.5 border-t-[0.5px] border-border-strong pt-5">
                <span className="font-serif text-lg text-accent-text tabular-nums">{String(index + 1).padStart(2, '0')}</span>
                <h3 className="m-0 text-[19px] font-medium">{t(`home.steps.items.${key}.title`)}</h3>
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
