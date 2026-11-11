import { Menu, PanelLeftClose, PanelLeftOpen } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { Link, useRouterState } from '@tanstack/react-router'
import { SiteBrand, useSiteBrand } from '@/components/SiteBrand'
import { createContext, Suspense, useContext, useState, type ReactNode, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Button } from './button'
import { Drawer } from './drawer'
import { Skeleton } from './layout'
import { RouteProgress } from './route-progress'
import { Tooltip, TooltipProvider } from './tooltip'

const Collapsed = createContext(false)
const sidebarKey = 'vp-sidebar-collapsed'
const textTransition = 'min-w-0 overflow-hidden whitespace-nowrap transition-[max-width,opacity] duration-200 motion-reduce:transition-none'
const sidebarRow = 'flex min-h-11 items-center overflow-hidden rounded-control px-3 text-label transition-colors hover:bg-hover hover:text-text'
const sidebarActive = 'bg-selected font-[500] text-text'

function sidebarCollapsed() {
  try { return localStorage.getItem(sidebarKey) === 'true' }
  catch { return false }
}

function PortalSkeleton() {
  const { t } = useTranslation()
  return (
    <div role="status" aria-label={t('polish.loading')} className="mx-auto flex max-w-[1200px] flex-col gap-5">
      <Skeleton className="h-9 w-40 max-w-full" />
      <Skeleton className="h-5 w-64 max-w-full" />
      <div className="grid gap-4 sm:grid-cols-3">
        <Skeleton className="h-24" /><Skeleton className="h-24" /><Skeleton className="h-24" />
      </div>
      <Skeleton className="h-64" />
    </div>
  )
}

export function PortalShell({ logo, navigation, userBlock, controls, problem, children, open, onOpenChange, menu, title }: {
  logo: ReactNode; navigation: ReactNode; userBlock: ReactNode; controls: ReactNode; problem?: ReactNode; children: ReactNode
  open: boolean; onOpenChange: (value: boolean) => void; menu: RefObject<HTMLButtonElement | null>; title: string
}) {
  const { t } = useTranslation()
  const [collapsed, setCollapsed] = useState(sidebarCollapsed)
  const pending = useRouterState({ select: (state) => state.status === 'pending' &&
    state.location.pathname !== state.resolvedLocation?.pathname })
  const CollapseIcon = collapsed ? PanelLeftOpen : PanelLeftClose
  const collapseLabel = t(collapsed ? 'ui.expandSidebar' : 'ui.collapseSidebar')

  function toggleSidebar() {
    const next = !collapsed
    setCollapsed(next)
    try { localStorage.setItem(sidebarKey, String(next)) }
    catch { /* The in-memory setting still applies. */ }
  }

  return (
    <TooltipProvider delayDuration={200}>
      <div className="min-h-dvh min-w-0 lg:flex">
        <aside className={cn('hidden shrink-0 flex-col gap-7 border-r-[0.5px] border-border bg-sidebar px-3 py-6 transition-[width] duration-200 motion-reduce:transition-none lg:sticky lg:top-0 lg:flex lg:h-dvh lg:overflow-x-hidden lg:overflow-y-auto', collapsed ? 'w-16' : 'w-56')}>
          <Collapsed.Provider value={collapsed}>
            <div className="shrink-0">{logo}</div>
            {navigation && <div className="min-h-0 overflow-y-auto">{navigation}</div>}
            <div className="mt-auto flex shrink-0 flex-col gap-2">
              <Tooltip enabled={collapsed} label={collapseLabel}>
                <button type="button" className={`${sidebarRow} text-text-2`}
                  aria-label={collapseLabel} aria-expanded={!collapsed} onClick={toggleSidebar}>
                  <CollapseIcon className="size-4 shrink-0" strokeWidth={1.6} aria-hidden="true" />
                  <span aria-hidden="true" className={cn(textTransition, collapsed ? 'max-w-0 opacity-0' : 'ml-3 max-w-40')}>
                    {t('ui.collapseSidebar')}
                  </span>
                </button>
              </Tooltip>
              {userBlock}
            </div>
          </Collapsed.Provider>
        </aside>
        <div className="min-w-0 flex-1">
          <header className="flex min-h-20 items-center justify-between gap-2 border-b-[0.5px] border-border px-4 py-3 sm:px-8 lg:justify-end lg:border-0">
            <div className="min-w-0 lg:hidden">{logo}</div>
            <Button ref={menu} className="lg:hidden" size="icon" variant="ghost" aria-label={t('ui.menu')}
              aria-expanded={open} onClick={() => onOpenChange(true)}>
              <Menu aria-hidden="true" />
            </Button>
            <div className="hidden items-center gap-2 lg:flex">{controls}</div>
          </header>
          <RouteProgress active={pending} />
          {problem && <div className="px-4 pt-4 sm:px-8">{problem}</div>}
          <main id="main" className="min-w-0 px-4 py-8 sm:px-8 lg:px-12 lg:py-10">
            <Suspense fallback={<PortalSkeleton />}>{children}</Suspense>
          </main>
        </div>
        <Drawer returnFocus={menu} open={open} onClose={() => onOpenChange(false)} title={title}>
          {navigation}{userBlock}
          <div className="flex flex-wrap items-center gap-2 border-t-[0.5px] border-border pt-4">{controls}</div>
        </Drawer>
      </div>
    </TooltipProvider>
  )
}

export function PortalBrand({ to, tag, onClose }: { to: string; tag?: string; onClose: () => void }) {
  const { name } = useSiteBrand()
  const collapsed = useContext(Collapsed)
  return (
    // The tag sits under the name so a 20px serif name keeps the full sidebar width;
    // pl-1.5 centres the 28px logo on the 16px nav icons, expanded and collapsed.
    <Link to={to} aria-label={tag ? `${name} · ${tag}` : name} onClick={onClose}
      className={cn('grid min-h-11 min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center overflow-hidden rounded-control pl-1.5',
        tag && '[&>:first-child]:row-span-2', collapsed ? '[&>span]:max-w-0 [&>span]:opacity-0' : 'gap-x-2.5 [&>span]:max-w-40')}>
      <SiteBrand labelClassName={cn(textTransition, tag && 'leading-tight')} />
      {tag && <span className={cn(textTransition, 'text-caption leading-tight text-text-3')}>{tag}</span>}
    </Link>
  )
}

export function PortalNavigation({ title, items, onClose }: { title: string; items: { path: string; label: string; icon: LucideIcon; exact?: boolean; badge?: ReactNode }[]; onClose: () => void }) {
  const collapsed = useContext(Collapsed)
  return (
    <nav aria-label={title} className="flex shrink-0 flex-col gap-0.5">
      {items.map((item) => {
        const link = (
          <Link key={item.path} to={item.path} aria-label={collapsed ? item.label : undefined} onClick={onClose}
            activeOptions={{ exact: item.exact ?? true, includeSearch: false }}
            activeProps={{ 'aria-current': 'page', className: sidebarActive }} inactiveProps={{ className: 'text-text-2' }}
            className={sidebarRow}>
            <item.icon className="size-4 shrink-0" strokeWidth={1.6} aria-hidden="true" />
            <span className={cn(textTransition, collapsed ? 'max-w-0 opacity-0' : 'ml-3 max-w-40')}>{item.label}</span>
            {!collapsed && item.badge}
          </Link>
        )
        return <Tooltip key={item.path} enabled={collapsed} label={<>{item.label}{item.badge}</>}>{link}</Tooltip>
      })}
    </nav>
  )
}

export function PortalUser({ name, role, to, onClose, mono = false }: { name: string; role: string; to?: string; onClose: () => void; mono?: boolean }) {
  const collapsed = useContext(Collapsed)
  const roleClassName = mono ? 'block font-mono tabular-nums text-text-3' : 'block text-text-3'
  const content = (
    <>
      <span aria-hidden="true" className="grid size-8 shrink-0 place-items-center rounded-full bg-oat font-serif text-label text-text">{name.slice(0, 1)}</span>
      <span className={cn(textTransition, collapsed ? 'max-w-0 opacity-0' : 'ml-2.5 max-w-full')}>
        <span className="block truncate text-label" title={name}>{name}</span>
        <span className={`text-caption ${roleClassName}`}>{role}</span>
      </span>
    </>
  )
  const className = cn('flex min-h-11 min-w-0 items-center overflow-hidden rounded-control py-3', collapsed ? 'px-1' : 'px-2')
  const link = to
    ? <Link to={to} aria-label={name} onClick={onClose} className={cn(className, 'hover:bg-hover')}>{content}</Link>
    : <div tabIndex={collapsed ? 0 : undefined} aria-label={collapsed ? name : undefined} className={className}>{content}</div>
  return (
    <div className="mt-auto shrink-0 border-t-[0.5px] border-border pt-4 lg:pt-2">
      <Tooltip enabled={collapsed} label={<>{name}<span className={roleClassName}>{role}</span></>}>{link}</Tooltip>
    </div>
  )
}
