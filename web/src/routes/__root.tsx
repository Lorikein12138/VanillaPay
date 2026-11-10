import { createRootRoute, Link, Outlet, useMatch, useRouter, type ErrorComponentProps } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ErrorState } from '@/components/ErrorState'
import { SiteIcons, SiteTitle } from '@/components/SiteBrand'
import { TopBar } from '@/components/TopBar'

export const Route = createRootRoute({
  component: Root,
  notFoundComponent: NotFound,
  errorComponent: RouteError,
})

function Root() {
  const checkout = useMatch({ from: '/pay/$tradeNo', shouldThrow: false })
  return <>{!checkout && <><SiteTitle /><SiteIcons /></>}<Outlet /></>
}

const actionClass =
  'inline-flex h-10 items-center rounded-control bg-primary px-4 text-label font-medium text-primary-fg transition-opacity hover:opacity-90'

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
