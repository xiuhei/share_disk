import { useState } from 'react'
import { 
  User, 
  Lock, 
  Bell, 
  Palette, 
  HardDrive, 
  Save,
  LogOut,
  Moon,
  Sun
} from 'lucide-react'
import { useTheme } from '../../contexts/ThemeContext'
import { useAuth } from '../../contexts/AuthContext'

export default function SettingsPage() {
  const { isDark, toggle } = useTheme()
  const { user, logout } = useAuth()
  const [activeSection, setActiveSection] = useState('account')

  const sections = [
    { key: 'account', label: '账户设置', icon: User },
    { key: 'security', label: '安全设置', icon: Lock },
    { key: 'notifications', label: '通知设置', icon: Bell },
    { key: 'appearance', label: '外观设置', icon: Palette },
    { key: 'storage', label: '存储管理', icon: HardDrive },
  ]

  return (
    <div className="h-full flex flex-col">
      {/* Page Header */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900 dark:text-white">设置</h1>
        <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
          管理您的账户和应用设置
        </p>
      </div>

      <div className="flex-1 flex gap-6 overflow-hidden">
        {/* Sidebar */}
        <div className="w-48 flex-shrink-0 hidden sm:block">
          <nav className="space-y-1">
            {sections.map(({ key, label, icon: Icon }) => (
              <button
                key={key}
                onClick={() => setActiveSection(key)}
                className={`w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
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
        <div className="flex-1 overflow-auto">
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
                      defaultValue={user?.username || ''}
                      className="input"
                    />
                  </div>
                  <div>
                    <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                      邮箱
                    </label>
                    <input
                      type="email"
                      placeholder="请输入邮箱"
                      className="input"
                    />
                  </div>
                  <button className="btn btn-primary">
                    <Save size={16} />
                    保存更改
                  </button>
                </div>
              </div>

              <div className="card border-red-200 dark:border-red-800">
                <h3 className="text-lg font-semibold text-red-600 dark:text-red-400 mb-2">危险操作</h3>
                <p className="text-sm text-gray-500 dark:text-gray-400 mb-4">
                  退出登录将清除本地缓存
                </p>
                <button onClick={logout} className="btn btn-danger">
                  <LogOut size={16} />
                  退出登录
                </button>
              </div>
            </div>
          )}

          {activeSection === 'security' && (
            <div className="card">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">安全设置</h3>
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                    当前密码
                  </label>
                  <input
                    type="password"
                    className="input"
                    placeholder="请输入当前密码"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                    新密码
                  </label>
                  <input
                    type="password"
                    className="input"
                    placeholder="请输入新密码"
                  />
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                    确认新密码
                  </label>
                  <input
                    type="password"
                    className="input"
                    placeholder="请再次输入新密码"
                  />
                </div>
                <button className="btn btn-primary">
                  <Save size={16} />
                  更新密码
                </button>
              </div>
            </div>
          )}

          {activeSection === 'notifications' && (
            <div className="card">
              <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">通知设置</h3>
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
                      <input type="checkbox" defaultChecked className="sr-only peer" />
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

          {activeSection === 'storage' && (
            <div className="space-y-6">
              <div className="card">
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">存储空间</h3>
                <div className="mb-4">
                  <div className="flex items-center justify-between text-sm text-gray-500 dark:text-gray-400 mb-2">
                    <span>已使用 3.5 GB</span>
                    <span>总计 10 GB</span>
                  </div>
                  <div className="w-full h-3 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                    <div className="h-full bg-primary-400 rounded-full" style={{ width: '35%' }} />
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div className="p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
                    <div className="text-gray-500 dark:text-gray-400">文档</div>
                    <div className="font-medium text-gray-900 dark:text-white">1.2 GB</div>
                  </div>
                  <div className="p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
                    <div className="text-gray-500 dark:text-gray-400">图片</div>
                    <div className="font-medium text-gray-900 dark:text-white">1.5 GB</div>
                  </div>
                  <div className="p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
                    <div className="text-gray-500 dark:text-gray-400">视频</div>
                    <div className="font-medium text-gray-900 dark:text-white">0.6 GB</div>
                  </div>
                  <div className="p-3 bg-gray-50 dark:bg-gray-700/50 rounded-lg">
                    <div className="text-gray-500 dark:text-gray-400">其他</div>
                    <div className="font-medium text-gray-900 dark:text-white">0.2 GB</div>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
