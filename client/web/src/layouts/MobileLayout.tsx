import { ReactNode, useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { ArrowUpDown, FolderOpen, HardDrive, Menu, Monitor, Search, Settings, Share2, Trash2, X } from 'lucide-react'
import { useTheme } from '../contexts/ThemeContext'

const primaryNav = [
  { path: '/files', icon: FolderOpen, label: '文件' },
  { path: '/transfers', icon: ArrowUpDown, label: '传输' },
  { path: '/devices', icon: Monitor, label: '设备' },
  { path: '/shares', icon: Share2, label: '分享' },
  { path: '/settings', icon: Settings, label: '设置' },
]

const titles: Record<string, string> = {
  '/files': '全部文件', '/transfers': '传输任务', '/devices': '设备',
  '/shares': '我的分享', '/trash': '回收站', '/settings': '设置',
}

export default function MobileLayout({ children }: { children: ReactNode }) {
  const [menuOpen, setMenuOpen] = useState(false)
  const { isDark, toggle } = useTheme()
  const { pathname } = useLocation()

  return (
    <div className="flex h-[100dvh] flex-col overflow-hidden" style={{ background: 'var(--bg)' }}>
      <header className="app-surface z-30 flex h-16 shrink-0 items-center justify-between border-b px-4">
        <div className="flex items-center gap-3">
          <span className="flex h-9 w-9 items-center justify-center rounded-xl bg-primary-600 text-white"><HardDrive size={18} /></span>
          <div><div className="text-[11px] font-medium uppercase tracking-[.14em] text-primary-600 dark:text-primary-400">Share Disk</div><div className="font-semibold leading-tight">{titles[pathname] || '空间'}</div></div>
        </div>
        <div className="flex items-center gap-1">
          <button className="icon-button" aria-label="搜索"><Search size={20} /></button>
          <button onClick={() => setMenuOpen(v => !v)} className="icon-button" aria-expanded={menuOpen} aria-label="更多菜单">{menuOpen ? <X size={20} /> : <Menu size={20} />}</button>
        </div>
      </header>

      {menuOpen && (
        <div className="app-surface absolute inset-x-3 top-[72px] z-20 rounded-2xl border p-2 shadow-xl">
          <NavLink to="/trash" onClick={() => setMenuOpen(false)} className="flex min-h-12 items-center gap-3 rounded-xl px-3 text-sm font-medium hover:bg-[var(--surface-soft)]"><Trash2 size={19} className="app-muted" />回收站</NavLink>
          <button onClick={toggle} className="flex min-h-12 w-full items-center gap-3 rounded-xl px-3 text-left text-sm font-medium hover:bg-[var(--surface-soft)]"><span className="app-muted flex w-[19px] justify-center">{isDark ? '☀' : '☾'}</span>{isDark ? '使用浅色模式' : '使用深色模式'}</button>
        </div>
      )}

      <main className="min-h-0 flex-1 overflow-auto p-4 pb-6">{children}</main>
      <nav className="app-surface safe-bottom z-20 grid min-h-[68px] shrink-0 grid-cols-5 border-t" aria-label="移动端导航">
        {primaryNav.map(({ path, icon: Icon, label }) => (
          <NavLink key={path} to={path} className={({ isActive }) => `flex min-w-0 flex-col items-center justify-center gap-1 text-[11px] font-medium transition-colors ${isActive ? 'text-primary-600 dark:text-primary-400' : 'app-muted'}`}>
            {({ isActive }) => <><span className={`flex h-8 w-12 items-center justify-center rounded-xl ${isActive ? 'bg-primary-50 dark:bg-primary-900/50' : ''}`}><Icon size={20} strokeWidth={isActive ? 2.2 : 1.8} /></span><span>{label}</span></>}
          </NavLink>
        ))}
      </nav>
    </div>
  )
}
