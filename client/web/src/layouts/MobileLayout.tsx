import { ReactNode } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { 
  FolderOpen, 
  ArrowUpDown, 
  Monitor, 
  Settings,
  Menu,
  X
} from 'lucide-react'
import { useState } from 'react'
import { useTheme } from '../contexts/ThemeContext'

const navItems = [
  { path: '/files', icon: FolderOpen, label: '文件' },
  { path: '/transfers', icon: ArrowUpDown, label: '传输' },
  { path: '/devices', icon: Monitor, label: '设备' },
  { path: '/settings', icon: Settings, label: '设置' },
]

interface MobileLayoutProps {
  children: ReactNode
}

export default function MobileLayout({ children }: MobileLayoutProps) {
  const [menuOpen, setMenuOpen] = useState(false)
  const { isDark, toggle } = useTheme()

  return (
    <div className="flex flex-col h-screen bg-gray-50 dark:bg-gray-900">
      {/* Header */}
      <header className="h-14 flex items-center justify-between px-4 border-b border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800">
        <div className="flex items-center gap-3">
          <div className="w-8 h-8 rounded-lg bg-primary-400 flex items-center justify-center">
            <span className="text-white font-bold text-sm">S</span>
          </div>
          <h1 className="text-lg font-bold text-gray-900 dark:text-white">Share Disk</h1>
        </div>
        <button
          onClick={() => setMenuOpen(!menuOpen)}
          className="p-2 text-gray-500 hover:text-gray-700 dark:text-gray-400 rounded-lg"
        >
          {menuOpen ? <X size={20} /> : <Menu size={20} />}
        </button>
      </header>

      {/* Dropdown Menu */}
      {menuOpen && (
        <div className="absolute top-14 left-0 right-0 bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 shadow-lg z-50">
          <div className="p-4 space-y-2">
            {navItems.map(({ path, icon: Icon, label }) => (
              <NavLink
                key={path}
                to={path}
                onClick={() => setMenuOpen(false)}
                className={({ isActive }) =>
                  `flex items-center gap-3 px-4 py-3 rounded-lg transition-colors ${
                    isActive
                      ? 'bg-primary-50 dark:bg-primary-900/20 text-primary-500'
                      : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700/50'
                  }`
                }
              >
                <Icon size={20} />
                <span>{label}</span>
              </NavLink>
            ))}
            <div className="border-t border-gray-200 dark:border-gray-700 pt-2">
              <button
                onClick={toggle}
                className="flex items-center gap-3 px-4 py-3 w-full text-left rounded-lg text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700/50"
              >
                {isDark ? '☀️' : '🌙'}
                <span>{isDark ? '浅色模式' : '深色模式'}</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Main Content */}
      <main className="flex-1 overflow-auto">
        {children}
      </main>

      {/* Bottom Navigation */}
      <nav className="h-16 flex items-center justify-around border-t border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800 safe-bottom">
        {navItems.map(({ path, icon: Icon, label }) => (
          <NavLink
            key={path}
            to={path}
            className={({ isActive }) =>
              `flex flex-col items-center gap-1 px-3 py-2 rounded-lg transition-colors ${
                isActive
                  ? 'text-primary-500'
                  : 'text-gray-500 dark:text-gray-400'
              }`
            }
          >
            <Icon size={22} />
            <span className="text-xs">{label}</span>
          </NavLink>
        ))}
      </nav>
    </div>
  )
}
