import { 
  Monitor, 
  Smartphone, 
  Wifi, 
  WifiOff, 
  Trash2, 
  RefreshCw,
  HardDrive,
  Clock
} from 'lucide-react'

interface Device {
  id: string
  name: string
  type: 'desktop' | 'mobile'
  online: boolean
  lastSeen: string
  storage: { used: number; total: number }
  ip?: string
}

const mockDevices: Device[] = [
  { 
    id: '1', 
    name: 'Ubuntu 工作站', 
    type: 'desktop', 
    online: true, 
    lastSeen: '刚刚',
    storage: { used: 15 * 1024 * 1024 * 1024, total: 50 * 1024 * 1024 * 1024 },
    ip: '192.168.1.100'
  },
  { 
    id: '2', 
    name: 'Android 手机', 
    type: 'mobile', 
    online: true, 
    lastSeen: '5 分钟前',
    storage: { used: 8 * 1024 * 1024 * 1024, total: 64 * 1024 * 1024 * 1024 },
    ip: '192.168.1.101'
  },
  { 
    id: '3', 
    name: 'MacBook Pro', 
    type: 'desktop', 
    online: false, 
    lastSeen: '2 天前',
    storage: { used: 25 * 1024 * 1024 * 1024, total: 256 * 1024 * 1024 * 1024 }
  },
]

function formatBytes(bytes: number): string {
  const gb = bytes / (1024 * 1024 * 1024)
  return `${gb.toFixed(1)} GB`
}

export default function DevicesPage() {
  return (
    <div className="h-full flex flex-col">
      {/* Page Header */}
      <div className="flex items-center justify-between mb-6">
        <div>
          <h1 className="text-2xl font-bold text-gray-900 dark:text-white">设备管理</h1>
          <p className="text-sm text-gray-500 dark:text-gray-400 mt-1">
            管理已连接的设备
          </p>
        </div>
        <button className="btn btn-secondary">
          <RefreshCw size={18} />
          <span className="hidden sm:inline">刷新</span>
        </button>
      </div>

      {/* Stats */}
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 mb-6">
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-green-100 dark:bg-green-900/20 flex items-center justify-center">
              <Wifi size={20} className="text-green-500" />
            </div>
            <div>
              <div className="text-2xl font-bold text-gray-900 dark:text-white">2</div>
              <div className="text-xs text-gray-500 dark:text-gray-400">在线设备</div>
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-gray-100 dark:bg-gray-700 flex items-center justify-center">
              <WifiOff size={20} className="text-gray-400" />
            </div>
            <div>
              <div className="text-2xl font-bold text-gray-900 dark:text-white">1</div>
              <div className="text-xs text-gray-500 dark:text-gray-400">离线设备</div>
            </div>
          </div>
        </div>
        <div className="card">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded-lg bg-primary-100 dark:bg-primary-900/20 flex items-center justify-center">
              <HardDrive size={20} className="text-primary-500" />
            </div>
            <div>
              <div className="text-2xl font-bold text-gray-900 dark:text-white">48 GB</div>
              <div className="text-xs text-gray-500 dark:text-gray-400">总存储空间</div>
            </div>
          </div>
        </div>
      </div>

      {/* Device List */}
      <div className="flex-1 overflow-auto space-y-3">
        {mockDevices.map(device => (
          <div
            key={device.id}
            className="card hover:border-gray-300 dark:hover:border-gray-600 transition-colors"
          >
            <div className="flex items-start gap-4">
              {/* Device Icon */}
              <div className={`w-12 h-12 rounded-xl flex items-center justify-center flex-shrink-0 ${
                device.online
                  ? 'bg-primary-100 dark:bg-primary-900/20'
                  : 'bg-gray-100 dark:bg-gray-700'
              }`}>
                {device.type === 'desktop' ? (
                  <Monitor size={20} className={device.online ? 'text-primary-500' : 'text-gray-400'} />
                ) : (
                  <Smartphone size={20} className={device.online ? 'text-primary-500' : 'text-gray-400'} />
                )}
              </div>

              {/* Info */}
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <h3 className="font-medium text-gray-900 dark:text-white">{device.name}</h3>
                  <span className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-xs font-medium ${
                    device.online
                      ? 'bg-green-100 dark:bg-green-900/20 text-green-700 dark:text-green-400'
                      : 'bg-gray-100 dark:bg-gray-700 text-gray-500 dark:text-gray-400'
                  }`}>
                    <span className={`w-1.5 h-1.5 rounded-full ${device.online ? 'bg-green-500' : 'bg-gray-400'}`} />
                    {device.online ? '在线' : '离线'}
                  </span>
                </div>
                
                <div className="flex items-center gap-4 text-sm text-gray-500 dark:text-gray-400 mt-1">
                  <span className="flex items-center gap-1">
                    <Clock size={14} />
                    {device.lastSeen}
                  </span>
                  {device.ip && <span>{device.ip}</span>}
                </div>

                {/* Storage Bar */}
                <div className="mt-3">
                  <div className="flex items-center justify-between text-xs text-gray-500 dark:text-gray-400 mb-1">
                    <span>存储空间</span>
                    <span>{formatBytes(device.storage.used)} / {formatBytes(device.storage.total)}</span>
                  </div>
                  <div className="w-full h-1.5 bg-gray-200 dark:bg-gray-700 rounded-full overflow-hidden">
                    <div
                      className="h-full bg-primary-400 rounded-full"
                      style={{ width: `${(device.storage.used / device.storage.total) * 100}%` }}
                    />
                  </div>
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
      </div>
    </div>
  )
}
