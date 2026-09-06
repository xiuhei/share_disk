import { useState } from 'react'
import { 
  Share2, 
  Link, 
  Copy, 
  Trash2, 
  Clock, 
  Shield,
  Plus,
  Check
} from 'lucide-react'
import { formatDate } from '../../utils/helpers'

interface ShareLink {
  id: string
  fileName: string
  url: string
  createdAt: string
  expiresAt: string
  downloads: number
  password?: string
}

const mockShares: ShareLink[] = [
  { 
    id: '1', 
    fileName: '项目报告.pdf', 
    url: 'https://share.example.com/s/abc123',
    createdAt: '2024-01-15',
    expiresAt: '2024-01-16',
    downloads: 5,
    password: '1234'
  },
  { 
    id: '2', 
    fileName: '设计稿.png', 
    url: 'https://share.example.com/s/def456',
    createdAt: '2024-01-14',
    expiresAt: '2024-01-15',
    downloads: 12
  },
  { 
    id: '3', 
    fileName: '演示视频.mp4', 
    url: 'https://share.example.com/s/ghi789',
    createdAt: '2024-01-13',
    expiresAt: '2024-01-14',
    downloads: 3
  },
]

export default function SharesPage() {
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [showCreateModal, setShowCreateModal] = useState(false)

  const handleCopy = async (url: string, id: string) => {
    await navigator.clipboard.writeText(url)
    setCopiedId(id)
    setTimeout(() => setCopiedId(null), 2000)
  }

  return (
    <div className="h-full flex flex-col">
      {/* Page Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">分享管理</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            管理已创建的分享链接
          </p>
        </div>
        <button 
          className="btn btn-primary"
          onClick={() => setShowCreateModal(true)}
        >
          <Plus size={18} />
          <span className="hidden sm:inline">创建链接</span>
        </button>
      </div>

      {/* Info Banner */}
      <div className="flex items-start gap-3 p-4 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-xl mb-6">
        <Shield size={20} className="text-blue-600 dark:text-blue-400 flex-shrink-0 mt-0.5" />
        <div>
          <p className="text-sm font-medium text-blue-800 dark:text-blue-300">
            分享链接安全提示
          </p>
          <p className="text-sm text-blue-700 dark:text-blue-400 mt-1">
            分享链接默认 24 小时有效，建议设置密码保护
          </p>
        </div>
      </div>

      {/* Share List */}
      <div className="flex-1 overflow-auto space-y-3">
        {mockShares.map(share => (
          <div
            key={share.id}
            className="card hover:border-gray-300 dark:hover:border-gray-600 transition-colors"
          >
            <div className="flex items-start gap-4">
              {/* Icon */}
              <div className="w-12 h-12 rounded-xl bg-primary-100 dark:bg-primary-900/20 flex items-center justify-center flex-shrink-0">
                <Share2 size={20} className="text-primary-500" />
              </div>

              {/* Info */}
              <div className="flex-1 min-w-0">
                <h3 className="font-medium text-gray-900 dark:text-white truncate">
                  {share.fileName}
                </h3>
                <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-gray-500 dark:text-gray-400 mt-1">
                  <span className="flex items-center gap-1">
                    <Clock size={14} />
                    创建于 {formatDate(share.createdAt)}
                  </span>
                  <span>过期于 {share.expiresAt}</span>
                  <span>{share.downloads} 次下载</span>
                  {share.password && (
                    <span className="flex items-center gap-1 text-yellow-600 dark:text-yellow-400">
                      <Shield size={14} />
                      有密码
                    </span>
                  )}
                </div>

                {/* URL */}
                <div className="flex items-center gap-2 mt-3">
                  <div className="flex-1 flex items-center gap-2 px-3 py-2 bg-gray-100 dark:bg-gray-700 rounded-lg text-sm text-gray-600 dark:text-gray-300 font-mono truncate">
                    <Link size={14} className="flex-shrink-0" />
                    <span className="truncate">{share.url}</span>
                  </div>
                  <button
                    onClick={() => handleCopy(share.url, share.id)}
                    className={`p-2 rounded-lg transition-colors ${
                      copiedId === share.id
                        ? 'bg-green-100 dark:bg-green-900/20 text-green-600 dark:text-green-400'
                        : 'bg-gray-100 dark:bg-gray-700 text-gray-500 hover:text-primary-500'
                    }`}
                  >
                    {copiedId === share.id ? <Check size={16} /> : <Copy size={16} />}
                  </button>
                </div>
              </div>

              {/* Actions */}
              <div className="flex items-center gap-1">
                <button className="p-2 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
                  <Trash2 size={16} />
                </button>
              </div>
            </div>
          </div>
        ))}

        {mockShares.length === 0 && (
          <div className="text-center py-12">
            <Share2 size={48} className="mx-auto text-gray-300 dark:text-gray-600 mb-4" />
            <p className="text-gray-500 dark:text-gray-400">暂无分享链接</p>
          </div>
        )}
      </div>

      {/* Create Modal */}
      {showCreateModal && (
        <div className="fixed inset-0 modal-backdrop z-50 flex items-center justify-center p-4">
          <div className="w-full max-w-md bg-white dark:bg-gray-800 rounded-2xl shadow-xl">
            <div className="p-6">
              <h2 className="text-lg font-semibold text-gray-900 dark:text-white mb-4">
                创建分享链接
              </h2>
              
              <div className="space-y-4">
                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                    选择文件
                  </label>
                  <select className="input">
                    <option>项目报告.pdf</option>
                    <option>设计稿.png</option>
                    <option>演示视频.mp4</option>
                  </select>
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                    有效期
                  </label>
                  <select className="input">
                    <option>24 小时</option>
                    <option>7 天</option>
                    <option>30 天</option>
                    <option>永久有效</option>
                  </select>
                </div>

                <div>
                  <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1.5">
                    访问密码（可选）
                  </label>
                  <input
                    type="text"
                    className="input"
                    placeholder="留空则无需密码"
                  />
                </div>
              </div>
            </div>

            <div className="flex items-center justify-end gap-3 px-6 py-4 border-t border-gray-200 dark:border-gray-700">
              <button
                onClick={() => setShowCreateModal(false)}
                className="btn btn-secondary"
              >
                取消
              </button>
              <button
                onClick={() => setShowCreateModal(false)}
                className="btn btn-primary"
              >
                创建链接
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
