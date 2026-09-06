import { 
  Trash2, 
  RotateCcw, 
  AlertTriangle,
  Clock,
  File,
  Image,
  Film,
  Music,
  FileText
} from 'lucide-react'
import { formatFileSize, getFileType } from '../../utils/helpers'

interface TrashItem {
  id: string
  name: string
  size: number
  type: string
  deletedAt: string
  expiresAt: string
}

const mockTrash: TrashItem[] = [
  { id: '1', name: '旧版本报告.pdf', size: 1500000, type: 'application/pdf', deletedAt: '2024-01-10', expiresAt: '2024-02-10' },
  { id: '2', name: '测试图片.jpg', size: 2500000, type: 'image/jpeg', deletedAt: '2024-01-08', expiresAt: '2024-02-08' },
  { id: '3', name: '备份数据.zip', size: 45000000, type: 'application/zip', deletedAt: '2024-01-05', expiresAt: '2024-02-05' },
  { id: '4', name: '草稿文档.docx', size: 85000, type: 'application/docx', deletedAt: '2024-01-03', expiresAt: '2024-02-03' },
]

function FileIcon({ type }: { type: string }) {
  const fileType = getFileType(type)
  const icons = {
    image: <Image size={20} className="text-purple-500" />,
    video: <Film size={20} className="text-red-500" />,
    audio: <Music size={20} className="text-yellow-500" />,
    document: <FileText size={20} className="text-blue-500" />,
    other: <File size={20} className="text-gray-500" />,
  }
  return icons[fileType]
}

export default function TrashPage() {
  return (
    <div className="h-full flex flex-col">
      {/* Page Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">回收站</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            已删除的文件将在 30 天后永久删除
          </p>
        </div>
        <button className="btn btn-danger">
          <Trash2 size={18} />
          <span className="hidden sm:inline">清空回收站</span>
        </button>
      </div>

      {/* Warning Banner */}
      <div className="flex items-start gap-3 p-4 bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-xl mb-6">
        <AlertTriangle size={20} className="text-yellow-600 dark:text-yellow-400 flex-shrink-0 mt-0.5" />
        <div>
          <p className="text-sm font-medium text-yellow-800 dark:text-yellow-300">
            回收站中的文件仍占用存储空间
          </p>
          <p className="text-sm text-yellow-700 dark:text-yellow-400 mt-1">
            您可以恢复文件或永久删除以释放空间
          </p>
        </div>
      </div>

      {/* Trash List */}
      <div className="flex-1 overflow-auto">
        {mockTrash.length > 0 ? (
          <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400">文件名</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 hidden sm:table-cell">大小</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 hidden md:table-cell">删除时间</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 hidden lg:table-cell">过期时间</th>
                  <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400">操作</th>
                </tr>
              </thead>
              <tbody>
                {mockTrash.map(item => (
                  <tr
                    key={item.id}
                    className="border-b border-gray-100 dark:border-gray-700/50 last:border-0 hover:bg-gray-50 dark:hover:bg-gray-700/30 transition-colors"
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-3">
                        <FileIcon type={item.type} />
                        <span className="font-medium text-gray-900 dark:text-white truncate">{item.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400 hidden sm:table-cell">
                      {formatFileSize(item.size)}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400 hidden md:table-cell">
                      {item.deletedAt}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400 hidden lg:table-cell">
                      {item.expiresAt}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        <button className="p-2 text-gray-400 hover:text-green-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors" title="恢复">
                          <RotateCcw size={16} />
                        </button>
                        <button className="p-2 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors" title="永久删除">
                          <Trash2 size={16} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <div className="text-center py-12">
            <Trash2 size={48} className="mx-auto text-gray-300 dark:text-gray-600 mb-4" />
            <p className="text-gray-500 dark:text-gray-400">回收站为空</p>
          </div>
        )}
      </div>
    </div>
  )
}
