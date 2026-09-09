import { useState } from 'react'
import { 
  Trash2, 
  RotateCcw, 
  File,
  Image,
  Film,
  Music,
  FileText
} from 'lucide-react'
import { formatFileSize, getFileType } from '../../utils/helpers'
import ConfirmDialog from '../../components/ConfirmDialog'

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
  const [items, setItems] = useState(mockTrash)
  const [pendingDelete, setPendingDelete] = useState<TrashItem | 'all' | null>(null)

  const confirmDelete = () => {
    if (pendingDelete === 'all') setItems([])
    else if (pendingDelete) setItems(current => current.filter(item => item.id !== pendingDelete.id))
    setPendingDelete(null)
  }

  return (
    <div className="page-shell">
      {/* Page Header */}
      <div className="mb-6 flex items-center justify-end">
        <button disabled={items.length === 0} onClick={() => setPendingDelete('all')} className="btn btn-danger">
          <Trash2 size={18} />
          <span className="hidden sm:inline">清空回收站</span>
        </button>
      </div>

      {/* Trash List */}
      <div className="flex-1 overflow-auto">
        {items.length > 0 ? (
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
                {items.map(item => (
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
                        <button onClick={() => setItems(current => current.filter(entry => entry.id !== item.id))} className="p-2 text-gray-400 hover:text-green-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors" title="恢复" aria-label={`恢复 ${item.name}`}>
                          <RotateCcw size={16} />
                        </button>
                        <button onClick={() => setPendingDelete(item)} className="p-2 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors" title="永久删除" aria-label={`永久删除 ${item.name}`}>
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
      <ConfirmDialog
        open={pendingDelete !== null}
        title={pendingDelete === 'all' ? '清空回收站？' : `永久删除“${pendingDelete?.name || ''}”？`}
        confirmLabel={pendingDelete === 'all' ? '确认清空' : '永久删除'}
        destructive
        onCancel={() => setPendingDelete(null)}
        onConfirm={confirmDelete}
      />
    </div>
  )
}
