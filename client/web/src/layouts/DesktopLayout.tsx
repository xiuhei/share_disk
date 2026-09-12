import { ReactNode, useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../contexts/AuthContext'
import { NavLink, useLocation, useNavigate } from 'react-router-dom'
import {
  ArrowUpDown, FolderOpen, HardDrive, Monitor, RefreshCw,
  Search, Settings, Share2, Trash2
} from 'lucide-react'

const navGroups = [
  {
    label: '工作区',
    items: [
      { path: '/files', icon: FolderOpen, label: '全部文件' },
      { path: '/transfers', icon: ArrowUpDown, label: '传输任务' },
      { path: '/shares', icon: Share2, label: '我的分享' },
    ],
  },
  {
    label: '系统',
    items: [
      { path: '/devices', icon: Monitor, label: '设备' },
      { path: '/trash', icon: Trash2, label: '回收站' },
      { path: '/settings', icon: Settings, label: '设置' },
    ],
  },
]

const titles: Record<string, string> = {
  '/files': '我的文件', '/transfers': '传输任务', '/shares': '我的分享',
  '/devices': '设备', '/trash': '回收站', '/settings': '设置',
}

export default function DesktopLayout({ children }: { children: ReactNode }) {
  const location = useLocation()
  const cache = useQueryClient()
  const { user } = useAuth()
  const navigate = useNavigate()
  const [searchOpen, setSearchOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [refreshState, setRefreshState] = useState<'idle' | 'refreshing' | 'done'>('idle')
  const searchInput = useRef<HTMLInputElement>(null)
  const refreshTimer = useRef<number | null>(null)

  const publishSearch = (value: string) => {
    setQuery(value)
    window.dispatchEvent(new CustomEvent('share-disk-search', { detail: value }))
  }
  const toggleSearch = () => {
    if (!searchOpen) {
      if (location.pathname !== '/files') navigate('/files')
      setSearchOpen(true)
      window.setTimeout(() => searchInput.current?.focus(), 0)
      return
    }
    setSearchOpen(false)
    publishSearch('')
  }

  const refresh = async () => {
    if (refreshState === 'refreshing') return
    setRefreshState('refreshing')
    await cache.invalidateQueries({ queryKey: [user?.id] })
    setRefreshState('done')
    refreshTimer.current = window.setTimeout(() => setRefreshState('idle'), 1400)
  }

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.ctrlKey && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        if (location.pathname !== '/files') navigate('/files')
        setSearchOpen(true)
        window.setTimeout(() => searchInput.current?.focus(), 0)
      } else if (event.key === 'Escape' && searchOpen) {
        setSearchOpen(false)
        publishSearch('')
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [location.pathname, navigate, searchOpen])

  useEffect(() => () => {
    if (refreshTimer.current !== null) window.clearTimeout(refreshTimer.current)
  }, [])

  return (
    <div className="flex h-screen overflow-hidden" style={{ background: 'var(--bg)' }}>
      <aside className="app-surface flex w-56 shrink-0 flex-col border-r px-3 py-4">
        <button onClick={() => navigate('/files')} className="mb-7 flex items-center gap-3 px-2 text-left" aria-label="返回全部文件">
          <div className="flex h-10 w-10 items-center justify-center rounded-[14px] bg-primary-600 text-white shadow-sm">
            <HardDrive size={21} strokeWidth={2} />
          </div>
          <div>
            <div className="font-semibold tracking-tight">Share Disk</div>
            <div className="app-muted text-xs">私人云空间</div>
          </div>
        </button>

        <nav className="flex-1 space-y-6" aria-label="主导航">
          {navGroups.map(group => (
            <div key={group.label}>
              <div className="app-muted mb-2 px-3 text-xs font-medium uppercase tracking-[.12em]">{group.label}</div>
              <div className="space-y-1">
                {group.items.map(({ path, icon: Icon, label }) => (
                  <NavLink key={path} to={path} className={({ isActive }) =>
                    `flex min-h-11 items-center gap-3 rounded-xl px-3 text-sm font-medium transition-colors ${
                      isActive ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300' : 'app-muted hover:bg-black/[.035] hover:text-gray-900 dark:hover:bg-white/[.05] dark:hover:text-white'
                    }`
                  }>
                    <Icon size={19} strokeWidth={1.8} />
                    <span className="flex-1">{label}</span>

                  </NavLink>
                ))}
              </div>
            </div>
          ))}
        </nav>

        <div className="rounded-2xl bg-[var(--surface-soft)] p-3.5"><p className="text-sm font-medium">{user?.username}</p><p className="app-muted mt-1 text-xs">文件保存在你的存储设备中</p></div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="app-surface flex h-[72px] shrink-0 items-center gap-5 border-b px-7">
          <div className="min-w-[120px] text-sm font-semibold">{titles[location.pathname] || 'Share Disk'}</div>
          <div className="ml-auto flex min-w-0 items-center gap-1">
            {searchOpen && <label className="relative mr-2 block w-[min(420px,38vw)]">
              <Search className="app-muted absolute left-3 top-1/2 -translate-y-1/2" size={17} />
              <input ref={searchInput} value={query} onChange={event => publishSearch(event.target.value)} type="search" placeholder="搜索文件名" className="input h-10 min-h-10 border-transparent bg-[var(--surface-soft)] pl-9" />
            </label>}
            <button onClick={toggleSearch} className={`icon-button ${searchOpen ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300' : ''}`} aria-label={searchOpen ? '关闭搜索' : '搜索'} aria-expanded={searchOpen} title="搜索"><Search size={20} /></button>
            <span className={`mr-1 text-xs font-medium transition-opacity ${refreshState === 'idle' ? 'pointer-events-none opacity-0' : 'app-muted opacity-100'}`} role="status" aria-live="polite">
              {refreshState === 'refreshing' ? '刷新中…' : '已刷新'}
            </span>
            <button onClick={refresh} disabled={refreshState === 'refreshing'} className="icon-button disabled:cursor-default disabled:opacity-70" aria-label={refreshState === 'refreshing' ? '正在刷新' : '刷新'} title="刷新">
              <RefreshCw size={20} className={refreshState === 'refreshing' ? 'animate-spin' : ''} />
            </button>
            <button onClick={() => navigate('/settings')} className={`icon-button ${location.pathname === '/settings' ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300' : ''}`} aria-label="设置" title="设置"><Settings size={20} /></button>
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-auto p-7">{children}</main>
      </div>
    </div>
  )
}
