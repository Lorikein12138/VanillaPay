import { Menu } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { LogoMark } from '@/components/Logo'
import type { ReactNode, RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from './button'
import { Drawer } from './drawer'

export function PortalShell({ logo, navigation, userBlock, controls, problem, children, open, onOpenChange, menu, title }: {
  logo: ReactNode; navigation: ReactNode; userBlock: ReactNode; controls: ReactNode; problem?: ReactNode; children: ReactNode
  open: boolean; onOpenChange: (value: boolean) => void; menu: RefObject<HTMLButtonElement | null>; title: string
}) {
  const { t } = useTranslation()
  return <div className="grid min-h-dvh min-w-0 grid-rows-[auto_1fr] lg:grid-cols-[224px_1fr] lg:grid-rows-1">
    <aside className="hidden min-w-0 flex-col gap-7 bg-sidebar px-4 py-6 lg:flex lg:min-h-dvh">{logo}{navigation}{userBlock}</aside>
    <div className="min-w-0"><header className="flex min-h-20 items-center justify-between gap-2 border-b-[0.5px] border-border px-4 py-3 lg:justify-end lg:border-0 sm:px-8"><div className="lg:hidden">{logo}</div><Button ref={menu} className="lg:hidden" size="icon" variant="ghost" aria-label={t('ui.menu')} aria-expanded={open} onClick={() => onOpenChange(true)}><Menu aria-hidden="true" /></Button><div className="hidden items-center gap-2 lg:flex">{controls}</div></header>
      {problem && <div className="px-4 pt-4 sm:px-8">{problem}</div>}
      <main id="main" className="min-w-0 px-4 py-8 sm:px-8 lg:px-12 lg:py-10">{children}</main>
    </div>
    <Drawer returnFocus={menu} open={open} onClose={() => onOpenChange(false)} title={title}>{navigation}{userBlock}<div className="flex flex-wrap items-center gap-2 border-t-[0.5px] border-border pt-4">{controls}</div></Drawer>
  </div>
}

export function PortalBrandTag({ children }: { children: ReactNode }) {
  return <span className="ml-auto text-caption text-text-3">{children}</span>
}

export function PortalBrand({ to, tag, onClose }: { to: string; tag?: string; onClose: () => void }) {
  const { t } = useTranslation()
  return <Link to={to} onClick={onClose} className="flex min-h-11 items-center gap-2.5 rounded-control px-2"><LogoMark /><span className="font-serif text-xl font-medium">{t('common.brand')}</span>{tag && <PortalBrandTag>{tag}</PortalBrandTag>}</Link>
}

export function PortalNavigation({ title, items, onClose }: { title: string; items: { path: string; label: string; icon: LucideIcon; exact?: boolean }[]; onClose: () => void }) {
  return <nav aria-label={title} className="flex flex-col gap-1">{items.map((item) => <Link key={item.path} to={item.path} onClick={onClose} activeOptions={{ exact: item.exact ?? true, includeSearch: false }} activeProps={{ 'aria-current': 'page', className: 'bg-hover text-text' }} inactiveProps={{ className: 'text-text-2' }} className="flex min-h-11 items-center gap-3 rounded-control px-3 text-label transition-colors hover:bg-hover hover:text-text"><item.icon className="size-4 shrink-0" strokeWidth={1.6} aria-hidden="true" />{item.label}</Link>)}</nav>
}

export function PortalUser({ name, role, to, onClose }: { name: string; role: string; to: string; onClose: () => void }) {
  return <div className="mt-auto border-t-[0.5px] border-border pt-4"><Link to={to} aria-label={name} onClick={onClose} className="flex min-h-11 min-w-0 items-center gap-3 rounded-control px-2 py-3 hover:bg-hover"><span aria-hidden="true" className="grid size-9 shrink-0 place-items-center rounded-full bg-oat text-label">{name.slice(0, 1)}</span><span className="min-w-0"><span className="block truncate text-label">{name}</span><span className="block text-caption text-text-3">{role}</span></span></Link></div>
}
