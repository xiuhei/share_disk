import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  Clock, FolderSearch, Info, Monitor, Network, Pencil, RefreshCw,
  Router, Server, Smartphone, Trash2, Wifi, WifiOff, X
} from 'lucide-react'
import ConfirmDialog from '../../components/ConfirmDialog'
import ResourceState from '../../components/ResourceState'
import api from '../../services/api'
import { useResource, useAction } from '../../hooks/useResource'

type ConnectionMode = 'lan' | 'server' | 'offline' | 'unknown'
interface Device {
  id: string
  name: string
  type: 'desktop' | 'mobile' | 'server'
  online: boolean
  lastSeen: string
  storage: { used: number; total: number }
  ip?: string
  connectionMode: ConnectionMode
  connectedPeer?: string
}

const connectionMeta = {
  unknown: { label: '未上报', icon: Network, tone: 'bg-gray-100 text-gray-500 dark:bg-gray-800' },
  lan: { label: '局域网', icon: Network, tone: 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300' },
  server: { label: '公网', icon: Router, tone: 'bg-violet-50 text-violet-700 dark:bg-violet-950/40 dark:text-violet-300' },
  offline: { label: '未连接', icon: WifiOff, tone: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400' },
}

function formatBytes(bytes: number) { return `${(bytes / 1024 ** 3).toFixed(1)} GB` }

export default function DevicesPage() {
  const navigate = useNavigate()
  const resource = useResource(['devices'], () => api.listDevices(), 15000)
  const action = useAction()
  const devices: Device[] = (resource.data || []).map(device => ({
    id: device.id, name: device.name, type: device.platform === 'android' ? 'mobile' : device.platform === 'server' ? 'server' : 'desktop',
    online: device.status === 'active' && !!device.last_seen_at && Date.now() - Date.parse(device.last_seen_at) < 120000,
    lastSeen: device.last_seen_at ? new Date(device.last_seen_at).toLocaleString() : '尚无心跳',
    storage: { used: 0, total: 0 }, connectionMode: 'unknown',
  }))
  const [pendingRemove, setPendingRemove] = useState<Device | null>(null)
  const [detailDevice, setDetailDevice] = useState<Device | null>(null)
  const [renaming, setRenaming] = useState<Device | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const refreshing = resource.isFetching
  const onlineCount = devices.filter(device => device.online).length
  const connectionCounts = useMemo(() => ({
    lan: devices.filter(device => device.connectionMode === 'lan').length,
    server: devices.filter(device => device.connectionMode === 'server').length,
    offline: devices.filter(device => !device.online).length,
    unknown: devices.filter(device => device.connectionMode === 'unknown').length,
  }), [devices])

  const refresh = () => { void resource.refetch() }
  const startRename = (device: Device) => { setRenaming(device); setRenameValue(device.name) }
  const saveRename = () => {
    const name = renameValue.trim()
    if (!renaming || !name) return
    if (action.isPending) return
    action.mutate(() => api.renameDevice(renaming.id, name), { onSuccess: () => setRenaming(null) })
  }

  return (
    <div className="page-shell">
      <ResourceState loading={resource.isPending} error={action.error || resource.error} retry={refresh} />
      <div className="mb-6 flex items-center justify-end">
        <div className="flex items-center gap-2"><span className={`app-muted text-xs transition-opacity ${refreshing ? 'opacity-100' : 'opacity-0'}`} role="status">刷新中…</span><button onClick={refresh} className="btn btn-secondary" disabled={refreshing}><RefreshCw size={18} className={refreshing ? 'animate-spin' : ''} /><span className="hidden sm:inline">刷新</span></button></div>
      </div>

      <div className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
        <div className="card flex items-center gap-3"><span className="flex h-10 w-10 items-center justify-center rounded-xl bg-green-100 dark:bg-green-900/30"><Wifi size={19} className="text-green-600" /></span><div><div className="text-xl font-bold">{onlineCount}</div><div className="app-muted text-xs">在线设备</div></div></div>
        {(['lan', 'server', 'offline'] as ConnectionMode[]).map(mode => { const meta = connectionMeta[mode]; const Icon = meta.icon; return <div key={mode} className="card flex items-center gap-3"><span className={`flex h-10 w-10 items-center justify-center rounded-xl ${meta.tone}`}><Icon size={19} /></span><div><div className="text-xl font-bold">{connectionCounts[mode]}</div><div className="app-muted text-xs">{meta.label}</div></div></div> })}
      </div>

      <div className="min-h-0 flex-1 space-y-3 overflow-auto pb-2">
        {devices.map(device => {
          const meta = connectionMeta[device.connectionMode]
          const DeviceIcon = device.type === 'mobile' ? Smartphone : device.type === 'server' ? Server : Monitor
          const ConnectionIcon = meta.icon
          return <article key={device.id} className="card transition-colors hover:border-primary-200 dark:hover:border-primary-800">
            <div className="flex items-start gap-3 sm:gap-4">
              <span className={`flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl ${device.online ? 'bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300' : 'bg-gray-100 text-gray-400 dark:bg-gray-800'}`}><DeviceIcon size={22} /></span>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2"><h2 className="truncate font-semibold">{device.name}</h2><span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-xs font-medium ${device.online ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300' : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'}`}><span className={`h-1.5 w-1.5 rounded-full ${device.online ? 'bg-green-500' : 'bg-gray-400'}`} />{device.online ? '在线' : '离线'}</span><span className={`hidden items-center gap-1.5 rounded-full px-2 py-1 text-xs font-semibold lg:inline-flex ${meta.tone}`}><ConnectionIcon size={13} />{meta.label}</span></div>
                <div className="app-muted mt-1 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs"><span className="flex items-center gap-1"><Clock size={13} />{device.lastSeen}</span>{device.ip && <span>{device.ip}</span>}</div>
                <div className={`mt-3 flex items-center gap-2 rounded-xl px-3 py-2 text-xs lg:hidden ${meta.tone}`}><ConnectionIcon size={16} /><span className="font-semibold">{meta.label}</span></div>
                <div className="mt-3"><div className="app-muted mb-1 flex justify-between text-xs"><span>存储空间（未上报）</span><span>{formatBytes(device.storage.used)} / {formatBytes(device.storage.total)}</span></div><div className="h-1.5 overflow-hidden rounded-full bg-gray-200 dark:bg-gray-700"><div className="h-full rounded-full bg-primary-500" style={{ width: `${device.storage.total ? device.storage.used / device.storage.total * 100 : 0}%` }} /></div></div>
                <div className="mt-4 flex flex-wrap gap-2">
                  <button onClick={() => navigate(`/files?device=${device.id}`)} className="btn btn-secondary min-h-10 px-3"><FolderSearch size={16} />查看文件</button>
                  <button onClick={() => startRename(device)} className="btn btn-ghost min-h-10 px-3"><Pencil size={16} />重命名</button>
                  <button onClick={() => setDetailDevice(device)} className="btn btn-ghost min-h-10 px-3"><Info size={16} />详情</button>
                </div>
              </div>
              <button onClick={() => setPendingRemove(device)} className="icon-button shrink-0 text-gray-400 hover:text-red-500" aria-label={`移除 ${device.name}`} title="移除设备"><Trash2 size={17} /></button>
            </div>
          </article>
        })}
      </div>

      {renaming && <div className="modal-backdrop fixed inset-0 z-50 flex items-end justify-center p-3 sm:items-center" onMouseDown={() => setRenaming(null)}><div className="modal-panel app-surface w-full max-w-md rounded-2xl border p-5 shadow-2xl" onMouseDown={event => event.stopPropagation()}><div className="mb-4 flex items-center justify-between"><h2 className="text-lg font-semibold">重命名设备</h2><button className="icon-button" onClick={() => setRenaming(null)}><X size={19} /></button></div><input autoFocus className="input" value={renameValue} onChange={event => setRenameValue(event.target.value)} onKeyDown={event => event.key === 'Enter' && saveRename()} /><div className="mt-5 flex justify-end gap-2"><button className="btn btn-secondary" onClick={() => setRenaming(null)}>取消</button><button className="btn btn-primary" onClick={saveRename} disabled={!renameValue.trim()}>保存</button></div></div></div>}
      {detailDevice && <div className="modal-backdrop fixed inset-0 z-50 flex items-end justify-center p-3 sm:items-center" onMouseDown={() => setDetailDevice(null)}><div className="modal-panel app-surface w-full max-w-md rounded-2xl border p-5 shadow-2xl" onMouseDown={event => event.stopPropagation()}><div className="mb-5 flex items-start justify-between"><div><h2 className="text-lg font-semibold">{detailDevice.name}</h2><p className="app-muted mt-1 text-sm">设备详情</p></div><button className="icon-button" onClick={() => setDetailDevice(null)}><X size={19} /></button></div><dl className="grid grid-cols-[96px_1fr] gap-y-3 text-sm"><dt className="app-muted">状态</dt><dd>{detailDevice.online ? '在线' : '离线'}</dd><dt className="app-muted">连接模式</dt><dd>{connectionMeta[detailDevice.connectionMode].label}</dd><dt className="app-muted">连接对象</dt><dd>{detailDevice.connectedPeer || 'Share Disk 服务器'}</dd><dt className="app-muted">最近活动</dt><dd>{detailDevice.lastSeen}</dd><dt className="app-muted">设备地址</dt><dd>{detailDevice.ip || '未上报'}</dd><dt className="app-muted">存储占用</dt><dd>{formatBytes(detailDevice.storage.used)} / {formatBytes(detailDevice.storage.total)}</dd></dl></div></div>}
      <ConfirmDialog open={pendingRemove !== null} title={`移除“${pendingRemove?.name || ''}”？`} description="该设备的会话将被撤销，设备上的唯一副本可能暂时无法访问。" confirmLabel="移除设备" destructive onCancel={() => setPendingRemove(null)} onConfirm={() => { if (pendingRemove && !action.isPending) action.mutate(() => api.removeDevice(pendingRemove.id), { onSuccess: () => setPendingRemove(null) }) }} />
    </div>
  )
}
