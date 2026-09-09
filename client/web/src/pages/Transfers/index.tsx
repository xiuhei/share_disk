import { useState } from 'react'
import { 
  Upload, 
  Download, 
  Pause, 
  Play, 
  X, 
  CheckCircle2,
  AlertCircle,
  Clock
} from 'lucide-react'
import { formatFileSize } from '../../utils/helpers'
import ConfirmDialog from '../../components/ConfirmDialog'

interface TransferTask {
  id: string
  name: string
  size: number
  progress: number
  status: 'uploading' | 'downloading' | 'paused' | 'completed' | 'error'
  type: 'upload' | 'download'
  speed?: number
}

const mockTransfers: TransferTask[] = [
  { id: '1', name: '设计稿.png', size: 3200000, progress: 75, status: 'uploading', type: 'upload', speed: 1250000 },
  { id: '2', name: '演示视频.mp4', size: 156000000, progress: 45, status: 'downloading', type: 'download', speed: 2500000 },
  { id: '3', name: '项目报告.pdf', size: 2548000, progress: 100, status: 'completed', type: 'upload' },
  { id: '4', name: '备份.zip', size: 89000000, progress: 30, status: 'paused', type: 'upload' },
  { id: '5', name: '录音.mp3', size: 8500000, progress: 100, status: 'completed', type: 'download' },
]

function StatusIcon({ status }: { status: TransferTask['status'] }) {
  const icons = {
    uploading: <Upload size={16} className="text-blue-500 animate-pulse" />,
    downloading: <Download size={16} className="text-green-500 animate-pulse" />,
    paused: <Pause size={16} className="text-yellow-500" />,
    completed: <CheckCircle2 size={16} className="text-green-500" />,
    error: <AlertCircle size={16} className="text-red-500" />,
  }
  return icons[status]
}

export default function TransfersPage() {
  const [activeTab, setActiveTab] = useState<'all' | 'upload' | 'download'>('all')
  const [transfers, setTransfers] = useState(mockTransfers)
  const [pendingCancel, setPendingCancel] = useState<TransferTask | null>(null)

  const filteredTransfers = transfers.filter(t =>
    activeTab === 'all' || t.type === activeTab
  )

  return (
    <div className="page-shell">
      {/* Tabs */}
      <div className="flex items-center gap-1 p-1 bg-gray-100 dark:bg-gray-800 rounded-lg mb-6 w-fit">
        {[
          { key: 'all', label: '全部' },
          { key: 'upload', label: '上传' },
          { key: 'download', label: '下载' },
        ].map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key as typeof activeTab)}
            className={`px-4 py-2 text-sm font-medium rounded-md transition-colors ${
              activeTab === tab.key
                ? 'bg-white dark:bg-gray-700 text-gray-900 dark:text-white shadow-sm'
                : 'text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-300'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Transfer List */}
      <div className="flex-1 overflow-auto space-y-3">
        {filteredTransfers.map(transfer => (
          <div
            key={transfer.id}
            className="card hover:border-gray-300 dark:hover:border-gray-600 transition-colors"
          >
            <div className="flex items-start gap-4">
              {/* Icon */}
              <div className={`w-12 h-12 rounded-xl flex items-center justify-center flex-shrink-0 ${
                transfer.type === 'upload'
                  ? 'bg-blue-100 dark:bg-blue-900/20'
                  : 'bg-green-100 dark:bg-green-900/20'
              }`}>
                {transfer.type === 'upload' ? (
                  <Upload size={20} className="text-blue-500" />
                ) : (
                  <Download size={20} className="text-green-500" />
                )}
              </div>

              {/* Info */}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <h3 className="font-medium text-gray-900 dark:text-white truncate">
                    {transfer.name}
                  </h3>
                  <StatusIcon status={transfer.status} />
                </div>
                <div className="flex items-center gap-2 text-sm text-gray-500 dark:text-gray-400 mt-1">
                  <span>{formatFileSize(transfer.size)}</span>
                  {transfer.speed && transfer.status !== 'completed' && (
                    <>
                      <span>·</span>
                      <span>{formatFileSize(transfer.speed)}/s</span>
                    </>
                  )}
                </div>

                {/* Progress Bar */}
                {transfer.status !== 'completed' && (
                  <div className="mt-3">
                    <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400 mb-1">
                      <span>{transfer.progress}%</span>
                      <span>
                        {transfer.status === 'paused' ? '已暂停' : 
                         transfer.status === 'error' ? '传输失败' : '传输中'}
                      </span>
                    </div>
                    <div className="w-full h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                      <div
                        className={`h-full rounded-full transition-all ${
                          transfer.status === 'error'
                            ? 'bg-red-500'
                            : transfer.status === 'paused'
                            ? 'bg-yellow-500'
                            : 'bg-primary-400'
                        }`}
                        style={{ width: `${transfer.progress}%` }}
                      />
                    </div>
                  </div>
                )}
              </div>

              {/* Actions */}
              <div className="flex items-center gap-1">
                {transfer.status === 'uploading' || transfer.status === 'downloading' ? (
                  <>
                    <button className="p-2 text-gray-400 hover:text-yellow-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
                      <Pause size={16} />
                    </button>
                    <button onClick={() => setPendingCancel(transfer)} className="p-2 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors" aria-label={`取消 ${transfer.name} 的传输`}>
                      <X size={16} />
                    </button>
                  </>
                ) : transfer.status === 'paused' ? (
                  <>
                    <button className="p-2 text-gray-400 hover:text-green-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors">
                      <Play size={16} />
                    </button>
                    <button onClick={() => setPendingCancel(transfer)} className="p-2 text-gray-400 hover:text-red-500 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors" aria-label={`取消 ${transfer.name} 的传输`}>
                      <X size={16} />
                    </button>
                  </>
                ) : null}
              </div>
            </div>
          </div>
        ))}

        {filteredTransfers.length === 0 && (
          <div className="text-center py-12">
            <Clock size={48} className="mx-auto text-gray-300 dark:text-gray-600 mb-4" />
            <p className="text-gray-500 dark:text-gray-400">暂无传输任务</p>
          </div>
        )}
      </div>
      <ConfirmDialog open={pendingCancel !== null} title={`取消“${pendingCancel?.name || ''}”的传输？`} confirmLabel="取消传输" destructive onCancel={() => setPendingCancel(null)} onConfirm={() => { if (pendingCancel) setTransfers(current => current.filter(transfer => transfer.id !== pendingCancel.id)); setPendingCancel(null) }} />
    </div>
  )
}
