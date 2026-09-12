import { useState } from 'react'
import {
  User,
  Bell,
  Palette,
  Save,
  LogOut,
  Moon,
  Sun
} from 'lucide-react'
import { useTheme } from '../../contexts/ThemeContext'
import { useAuth } from '../../contexts/AuthContext'
import api from '../../services/api'
import { useAction } from '../../hooks/useResource'
import ResourceState from '../../components/ResourceState'

export default function SettingsPage() {
  const { isDark, toggle } = useTheme()
  const { user, logout } = useAuth()
  const [activeSection, setActiveSection] = useState('account')
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [message, setMessage] = useState('')
  const action = useAction()

  const sections = [
    { key: 'account', label: '账户设置', icon: User },
    { key: 'notifications', label: '通知设置', icon: Bell },
    { key: 'appearance', label: '外观设置', icon: Palette },
  ]

  return (
    <div className="page-shell">
      <ResourceState error={action.error} />
      {message && <p role="status" className="mb-3 text-primary-700">{message}</p>}
      <div className="flex min-h-0 flex-1 flex-col gap-4 overflow-hidden sm:flex-row sm:gap-6">
        {/* Sidebar */}
        <div className="w-full flex-shrink-0 overflow-x-auto sm:w-48">
          <nav className="flex gap-1 sm:block sm:space-y-1">
            {sections.map(({ key, label, icon: Icon }) => (
              <button
                key={key}
                onClick={() => setActiveSection(key)}
                className={`flex shrink-0 items-center gap-2 rounded-xl px-3 py-2.5 text-sm font-medium transition-colors sm:w-full sm:gap-3 ${
                  activeSection === key
                    ? 'bg-primary-50 dark:bg-primary-900/20 text-primary-500'
                    : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700/50'
                }`}
              >
                <Icon size={18} />
                {label}
              </button>
            ))}
          </nav>
        </div>

        {/* Content */}
        <div className="min-h-0 flex-1 overflow-auto pb-2">
          {activeSection === 'account' && (
            <div className="space-y-6">
              <div className="card">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">账户信息</h3>
                <div className="space-y-4">
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                      用户名
                    </label>
                    <input
                      type="text"
                      value={user?.username || ''}
                      readOnly
                      className="input"
                    />
                  </div>
                  <label className="block text-sm font-medium">当前密码<input type="password" autoComplete="current-password" className="input mt-2" value={currentPassword} onChange={event => setCurrentPassword(event.target.value)} /></label>
                  <label className="block text-sm font-medium">新密码<input type="password" autoComplete="new-password" className="input mt-2" value={newPassword} onChange={event => setNewPassword(event.target.value)} placeholder="至少 8 位" /></label>
                  <button className="btn btn-primary" disabled={!currentPassword || newPassword.length < 8 || action.isPending} onClick={() => action.mutate(() => api.changePassword(currentPassword,newPassword), { onSuccess: () => { setCurrentPassword(''); setNewPassword(''); setMessage('密码已修改，其他会话已注销。') } })}><Save size={16} />修改密码</button>
                </div>
              </div>

              <div className="card border-red-200 dark:border-red-800">
                <h3 className="text-lg font-semibold text-red-600 dark:text-red-400 mb-2">危险操作</h3>
                <button onClick={logout} className="btn btn-danger">
                  <LogOut size={16} />
                  退出登录
                </button>
              </div>
            </div>
          )}

          {activeSection === 'notifications' && (
            <div className="card">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">通知设置</h3><p className="app-muted mb-3 text-sm">通知服务尚未启用。</p>
              <div className="space-y-4">
                {[
                  { label: '上传完成通知', description: '文件上传完成后发送通知' },
                  { label: '下载完成通知', description: '文件下载完成后发送通知' },
                  { label: '分享被访问通知', description: '分享链接被访问时发送通知' },
                  { label: '设备离线通知', description: '设备离线时发送通知' },
                ].map((item, i) => (
                  <div key={i} className="flex items-center justify-between py-3 border-b border-gray-100 dark:border-gray-700 last:border-0">
                    <div>
                      <div className="font-medium text-gray-900 dark:text-white">{item.label}</div>
                      <div className="text-sm text-gray-500 dark:text-gray-400">{item.description}</div>
                    </div>
                    <label className="relative inline-flex items-center cursor-pointer">
                      <input type="checkbox" disabled title="通知服务尚未启用" className="sr-only peer" />
                      <div className="w-11 h-6 bg-gray-200 dark:bg-gray-700 peer-focus:ring-2 peer-focus:ring-primary-400 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-400"></div>
                    </label>
                  </div>
                ))}
              </div>
            </div>
          )}

          {activeSection === 'appearance' && (
            <div className="card">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">外观设置</h3>
              <div className="space-y-4">
                <div className="flex items-center justify-between py-3">
                  <div className="flex items-center gap-3">
                    {isDark ? <Moon size={20} className="text-gray-400" /> : <Sun size={20} className="text-yellow-500" />}
                    <div>
                      <div className="font-medium text-gray-900 dark:text-white">深色模式</div>
                      <div className="text-sm text-gray-500 dark:text-gray-400">切换深色/浅色主题</div>
                    </div>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input
                      type="checkbox"
                      checked={isDark}
                      onChange={toggle}
                      className="sr-only peer"
                    />
                    <div className="w-11 h-6 bg-gray-200 dark:bg-gray-700 peer-focus:ring-2 peer-focus:ring-primary-400 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-0.5 after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all peer-checked:bg-primary-400"></div>
                  </label>
                </div>
              </div>
            </div>
          )}

        </div>
      </div>
    </div>
  )
}
