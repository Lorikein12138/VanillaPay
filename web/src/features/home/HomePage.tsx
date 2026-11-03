import { ArrowDown, BellRing, ChartColumn, CodeXml, CreditCard, QrCode, ShieldCheck, ShoppingCart, Smartphone, Target, type LucideIcon } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { LogoMark } from '@/components/Logo'
import { TopBar } from '@/components/TopBar'

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
  'hidden h-10 items-center rounded-control px-3 text-label text-text-2 transition-colors hover:bg-muted hover:text-text md:inline-flex'

export function HomePage() {
  const { t } = useTranslation()

  return (
    <>
      <TopBar>
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
                className="inline-flex h-12 items-center gap-2 rounded-[10px] bg-primary px-6 font-medium text-primary-fg transition-opacity hover:opacity-90"
              >
                {t('home.cta')}
                <ArrowDown className="size-4" strokeWidth={1.8} aria-hidden="true" />
              </a>
            </div>
          </div>

          <div className="flex min-w-0 flex-[1_1_420px] justify-center">
            <figure className="m-0 flex w-full max-w-[440px] flex-col gap-5 rounded-dialog border border-border bg-surface p-6 shadow-[0_1px_2px_rgba(20,20,19,0.04),0_12px_32px_rgba(20,20,19,0.06)] sm:p-7">
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

        <section id="features" className="scroll-mt-4 border-y border-border bg-surface">
          <div className="mx-auto flex max-w-[1200px] flex-col gap-12 px-4 py-20 sm:px-6 lg:py-24">
            <div className="flex max-w-[640px] flex-col gap-3">
              <h2 className="m-0 font-serif text-[clamp(1.875rem,3.5vw,2.5rem)] leading-[1.2] font-normal">
                {t('home.features.title')}
              </h2>
              <p className="m-0 text-[17px] leading-[1.7] text-text-2">{t('home.features.subtitle')}</p>
            </div>
            <ul className="m-0 grid list-none grid-cols-1 gap-px overflow-hidden rounded-card border border-border bg-border p-0 md:grid-cols-2 lg:grid-cols-3">
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
              <li key={key} className="flex min-w-0 flex-col gap-3.5 border-t border-text pt-5">
                <span className="font-serif text-lg text-accent-text tabular-nums">{String(index + 1).padStart(2, '0')}</span>
                <h3 className="m-0 text-[19px] font-medium">{t(`home.steps.items.${key}.title`)}</h3>
                <p className="m-0 text-[15px] leading-[1.7] text-text-2">{t(`home.steps.items.${key}.body`)}</p>
              </li>
            ))}
          </ol>
        </section>
      </main>

      <footer className="border-t border-border">
        <div className="mx-auto flex max-w-[1200px] items-center gap-2.5 px-4 py-10 text-[13px] text-text-2 sm:px-6">
          <LogoMark className="size-5" />
          <span>{t('common.brand')}</span>
        </div>
      </footer>
    </>
  )
}
