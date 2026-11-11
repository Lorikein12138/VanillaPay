import type { QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Link, Outlet, useMatch, useRouter, type ErrorComponentProps } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ErrorState } from '@/components/ErrorState'
import { SiteIcons, SiteTitle } from '@/components/SiteBrand'
import { TopBar } from '@/components/TopBar'

export const Route = createRootRouteWithContext<{ queryClient: QueryClient; hasMountedPortal: () => boolean }>()({
  component: Root,
  notFoundComponent: NotFound,
  errorComponent: RouteError,
})

function Root() {
  const checkout = useMatch({ from: '/pay/$tradeNo', shouldThrow: false })
  return <>{!checkout && <><SiteTitle /><SiteIcons /></>}<Outlet /></>
}

const actionClass =
  'inline-flex h-11 items-center rounded-control bg-primary px-5 text-label font-[460] text-primary-fg transition-colors hover:bg-primary-hover'

function NotFound() {
  const { t } = useTranslation()
  return (
    <>
      <TopBar />
      <main id="main">
        <ErrorState
          code={t('notFound.code')}
          title={t('notFound.title')}
          body={t('notFound.body')}
          action={
            <Link to="/" className={actionClass}>
              {t('common.backHome')}
            </Link>
          }
        />
      </main>
    </>
  )
}

function RouteError({ reset }: ErrorComponentProps) {
  const { t } = useTranslation()
  const router = useRouter()
  return (
    <>
      <TopBar />
      <main id="main">
        <ErrorState
          title={t('error.title')}
          body={t('error.body')}
          action={
            <button
              type="button"
              className={actionClass}
              onClick={() => {
                reset()
                void router.invalidate()
              }}
            >
              {t('error.retry')}
            </button>
          }
        />
      </main>
    </>
  )
}
