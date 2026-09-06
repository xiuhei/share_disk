import { useState } from 'react'
import { 
  Upload, 
  FolderPlus, 
  Grid, 
  List, 
  MoreVertical,
  File,
  Image,
  Film,
  Music,
  FileText,
  Download,
  Trash2,
  Share2,
  Edit2,
  ChevronRight
} from 'lucide-react'
import { formatFileSize, getFileType } from '../../utils/helpers'

interface FileItem {
  id: string
  name: string
  size: number
  type: string
  modified: string
  isFolder: boolean
}

const mockFiles: FileItem[] = [
  { id: '1', name: '文档', size: 0, type: '', modified: '2024-01-15', isFolder: true },
  { id: '2', name: '图片', size: 0, type: '', modified: '2024-01-14', isFolder: true },
  { id: '3', name: '视频', size: 0, type: '', modified: '2024-01-13', isFolder: true },
  { id: '4', name: '项目报告.pdf', size: 2548000, type: 'application/pdf', modified: '2024-01-12', isFolder: false },
  { id: '5', name: '会议记录.docx', size: 156000, type: 'application/docx', modified: '2024-01-11', isFolder: false },
  { id: '6', name: '设计稿.png', size: 3200000, type: 'image/png', modified: '2024-01-10', isFolder: false },
  { id: '7', name: '演示视频.mp4', size: 156000000, type: 'video/mp4', modified: '2024-01-09', isFolder: false },
  { id: '8', name: '背景音乐.mp3', size: 8500000, type: 'audio/mp3', modified: '2024-01-08', isFolder: false },
]

function FileIcon({ type, isFolder }: { type: string; isFolder: boolean }) {
  if (isFolder) return <FolderPlus size={20} className="text-primary-500" />
  
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

export default function FilesPage() {
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('grid')
  const [currentPath, setCurrentPath] = useState<string[]>([])
  const [selectedFiles, setSelectedFiles] = useState<string[]>([])

  return (
    <div className="h-full flex flex-col">
      {/* Page Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">文件</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            管理您的所有文件
          </p>
        </div>
        <div className="flex items-center gap-3">
          <button className="btn btn-secondary">
            <FolderPlus size={18} />
            <span className="hidden sm:inline">新建文件夹</span>
          </button>
          <button className="btn btn-primary">
            <Upload size={18} />
            <span className="hidden sm:inline">上传文件</span>
          </button>
        </div>
      </div>

      {/* Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400 mb-4">
        <button className="hover:text-primary-500 transition-colors">全部文件</button>
        {currentPath.map((folder, i) => (
          <span key={i} className="flex items-center gap-2">
            <ChevronRight size={14} />
            <button className="hover:text-primary-500 transition-colors">{folder}</button>
          </span>
        ))}
      </div>

      {/* Toolbar */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400">
          <span>{mockFiles.length} 个项目</span>
          {selectedFiles.length > 0 && (
            <span className="text-primary-500">· 已选择 {selectedFiles.length} 项</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setViewMode('grid')}
            className={`p-2 rounded-lg transition-colors ${
              viewMode === 'grid'
                ? 'bg-primary-100 dark:bg-primary-900/20 text-primary-500'
                : 'text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700'
            }`}
          >
            <Grid size={18} />
          </button>
          <button
            onClick={() => setViewMode('list')}
            className={`p-2 rounded-lg transition-colors ${
              viewMode === 'list'
                ? 'bg-primary-100 dark:bg-primary-900/20 text-primary-500'
                : 'text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700'
            }`}
          >
            <List size={18} />
          </button>
        </div>
      </div>

      {/* File List */}
      <div className="flex-1 overflow-auto">
        {viewMode === 'grid' ? (
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 gap-4">
            {mockFiles.map(file => (
              <div
                key={file.id}
                className={`card cursor-pointer hover:border-primary-300 dark:hover:border-primary-600 transition-all ${
                  selectedFiles.includes(file.id)
                    ? 'border-primary-400 dark:border-primary-500 bg-primary-50 dark:bg-primary-900/10'
                    : ''
                }`}
                onClick={() => {
                  if (file.isFolder) {
                    setCurrentPath([...currentPath, file.name])
                  } else {
                    setSelectedFiles(
                      selectedFiles.includes(file.id)
                        ? selectedFiles.filter(id => id !== file.id)
                        : [...selectedFiles, file.id]
                    )
                  }
                }}
              >
                <div className="aspect-square flex items-center justify-center bg-gray-50 dark:bg-gray-700/50 rounded-lg mb-3">
                  <FileIcon type={file.type} isFolder={file.isFolder} />
                </div>
                <div className="truncate font-medium text-gray-900 dark:text-white">{file.name}</div>
                {!file.isFolder && (
                  <div className="text-xs text-gray-500 dark:text-gray-400 mt-1">
                    {formatFileSize(file.size)}
                  </div>
                )}
              </div>
            ))}
          </div>
        ) : (
          <div className="bg-white dark:bg-gray-800 rounded-xl border border-gray-200 dark:border-gray-700 overflow-hidden">
            <table className="w-full">
              <thead>
                <tr className="border-b border-gray-200 dark:border-gray-700">
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400">名称</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 hidden sm:table-cell">大小</th>
                  <th className="text-left px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400 hidden md:table-cell">修改时间</th>
                  <th className="text-right px-4 py-3 text-xs font-medium text-gray-500 dark:text-gray-400">操作</th>
                </tr>
              </thead>
              <tbody>
                {mockFiles.map(file => (
                  <tr
                    key={file.id}
                    className="border-b border-gray-100 dark:border-gray-700/50 last:border-0 hover:bg-gray-50 dark:hover:bg-gray-700/30 transition-colors"
                  >
                    <td className="px-4 py-3">
                      <div className="flex items-center gap-3">
                        <FileIcon type={file.type} isFolder={file.isFolder} />
                        <span className="font-medium text-gray-900 dark:text-white truncate">{file.name}</span>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400 hidden sm:table-cell">
                      {file.isFolder ? '-' : formatFileSize(file.size)}
                    </td>
                    <td className="px-4 py-3 text-sm text-gray-500 dark:text-gray-400 hidden md:table-cell">
                      {file.modified}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-1">
                        {!file.isFolder && (
                          <button className="p-1.5 text-gray-400 hover:text-primary-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
                            <Download size={16} />
                          </button>
                        )}
                        <button className="p-1.5 text-gray-400 hover:text-primary-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
                          <Share2 size={16} />
                        </button>
                        <button className="p-1.5 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
                          <Trash2 size={16} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>

      {/* Drop Zone Overlay */}
      <div className="hidden" id="drop-zone">
        <div className="fixed inset-0 bg-primary-500/10 dark:bg-primary-400/10 border-2 border-dashed border-primary-400 z-50 flex items-center justify-center">
          <div className="text-center">
            <Upload size={48} className="mx-auto text-primary-500 mb-4" />
            <p className="text-lg font-medium text-primary-600 dark:text-primary-400">拖放文件到此处上传</p>
          </div>
        </div>
      </div>
    </div>
  )
}
