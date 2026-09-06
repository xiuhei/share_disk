import { ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { 
  FolderOpen, 
  ArrowUpDown, 
  Monitor, 
  Trash2, 
  Share2, 
  Settings,
  Search,
  Bell,
  User
} from 'lucide-react'
import { useTheme } from '../contexts/ThemeContext'

const navItems = [
  { path: '/files', icon: FolderOpen, label: '文件' },
  { path: '/transfers', icon: ArrowUpDown, label: '传输' },
  { path: '/devices', icon: Monitor, label: '设备' },
  { path: '/trash', icon: Trash2, label: '回收站' },
  { path: '/shares', icon: Share2, label: '分享' },
  { path: '/settings', icon: Settings, label: '设置' },
]

interface DesktopLayoutProps {
  children: ReactNode
}

export default function DesktopLayout({ children }: DesktopLayoutProps) {
  const { isDark, toggle } = useTheme()
  const location = useLocation()

  return (
    <div className="flex h-screen bg-gray-50 dark:bg-gray-900">
      {/* Sidebar */}
      <aside className="w-60 flex-shrink-0 border-r border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 flex flex-col">
        {/* Logo */}
        <div className="h-16 flex items-center gap-3 px-5 border-b border-gray-200 dark:border-gray-700">
          <div className="w-10 h-10 rounded-xl bg-primary-400 flex items-center justify-center">
            <span className="text-white font-bold text-lg">S</span>
          </div>
          <div>
            <h1 className="text-lg font-bold text-gray-900 dark:text-white">Share Disk</h1>
            <p className="text-xs text-gray-500 dark:text-gray-400">分布式云盘</p>
          </div>
        </div>

        {/* Navigation */}
        <nav className="flex-1 px-3 py-4 space-y-1">
          {navItems.map(({ path, icon: Icon, label }) => (
            <NavLink
              key={path}
              to={path}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2.5 rounded-lg transition-colors ${
                  isActive
                    ? 'bg-primary-50 dark:bg-primary-900/20 text-primary-500 dark:text-primary-400 font-medium'
                    : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700/50'
                }`
              }
            >
              <Icon size={20} />
              <span>{label}</span>
            </NavLink>
          ))}
        </nav>

        {/* Storage Info */}
        <div className="px-4 py-4 border-t border-gray-200 dark:border-gray-700">
          <div className="text-xs text-gray-500 dark:text-gray-400 mb-2">存储空间</div>
          <div className="w-full h-2 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
            <div className="h-full bg-primary-400 rounded-full" style={{ width: '35%' }} />
          </div>
          <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">3.5 GB / 10 GB</div>
        </div>
      </aside>

      {/* Main Content */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Header */}
        <header className="h-16 flex items-center justify-between px-6 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
          {/* Search */}
          <div className="flex-1 max-w-xl">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" size={18} />
              <input
                type="text"
                placeholder="搜索文件..."
                className="w-full pl-10 pr-4 py-2 bg-gray-100 dark:bg-gray-700 border-0 rounded-lg text-sm focus:ring-2 focus:ring-primary-400 focus:bg-white dark:focus:bg-gray-600 transition-all"
              />
            </div>
          </div>

          {/* Actions */}
          <div className="flex items-center gap-4 ml-4">
            <button className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
              <Bell size={20} />
            </button>
            <button
              onClick={toggle}
              className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
            >
              {isDark ? '☀️' : '🌙'}
            </button>
            <div className="w-px h-6 bg-gray-200 dark:bg-gray-700" />
            <button className="flex items-center gap-2 px-3 py-1.5 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
              <div className="w-8 h-8 rounded-full bg-primary-100 dark:bg-primary-900/30 flex items-center justify-center">
                <User size={16} className="text-primary-600 dark:text-primary-400" />
              </div>
              <span className="text-sm font-medium text-gray-700 dark:text-gray-300">用户</span>
            </button>
          </div>
        </header>

        {/* Page Content */}
        <main className="flex-1 overflow-auto p-6">
          {children}
        </main>
      </div>
    </div>
  )
}
