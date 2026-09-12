import { useState } from 'react'
import { ArrowDownToLine, RefreshCw, X } from 'lucide-react'
import api, { TransferEntry } from '../../services/api'
import { useAction, useResource } from '../../hooks/useResource'
import ResourceState from '../../components/ResourceState'
import ConfirmDialog from '../../components/ConfirmDialog'

const labels: Record<string, string> = { queued: '排队中', assigned: '已分配', discovering: '查找来源', connecting: '连接中', transferring: '传输中', verifying: '校验中', completed: '已完成', waiting_source: '等待在线副本', retry_wait: '等待重试', paused: '已暂停', canceled: '已取消', failed_permanent: '传输失败' }
const terminal = new Set(['completed', 'canceled', 'failed_permanent'])
export default function TransfersPage() {
  const resource = useResource(['transfers'], () => api.listTransfers(), 5000)
  const devices = useResource(['devices'], () => api.listDevices())
  const action = useAction()
  const [filter, setFilter] = useState('all')
  const [pending, setPending] = useState<TransferEntry | null>(null)
  const tasks = (resource.data || []).filter(task => filter === 'all' || (filter === 'active' ? !terminal.has(task.state) : terminal.has(task.state)))
  return <div className="page-shell">
    <ResourceState loading={resource.isPending} error={action.error || resource.error} retry={() => { void resource.refetch() }} />
    <div className="mb-6 flex items-center justify-between"><div className="flex rounded-xl bg-[var(--surface-soft)] p-1">{[['all', '全部'], ['active', '进行中'], ['finished', '已结束']].map(([value,label]) => <button key={value} className={`btn ${filter === value ? 'btn-secondary' : 'btn-ghost'}`} onClick={() => setFilter(value)}>{label}</button>)}</div><button className="icon-button" aria-label="刷新传输任务" onClick={() => { void resource.refetch() }}><RefreshCw size={18} /></button></div>
    <div className="min-h-0 flex-1 space-y-3 overflow-auto">{tasks.map(task => <article key={task.id} className="card flex items-start gap-4"><span className="rounded-xl bg-primary-50 p-3 text-primary-600"><ArrowDownToLine size={22} /></span><div className="min-w-0 flex-1"><h2 className="truncate font-semibold">副本任务 {task.id.slice(0,8)}</h2><p className="app-muted mt-1 text-sm">目标设备：{devices.data?.find(device => device.id === task.target_device_id)?.name || task.target_device_id}</p><p role="status" className="mt-3 text-sm font-medium">{labels[task.state] || task.state}</p><p className="app-muted mt-1 text-xs">尝试 {task.attempt} 次 · {new Date(task.created_at).toLocaleString()}</p></div>{!terminal.has(task.state) && <button className="icon-button text-red-600" aria-label="取消传输任务" onClick={() => setPending(task)}><X size={18} /></button>}</article>)}{!resource.isPending && !resource.error && !tasks.length && <p className="app-muted py-12 text-center">暂无传输任务</p>}</div>
    <ConfirmDialog open={!!pending} title="取消此副本任务？" description="目标设备停止继续传输，已完成的其他副本不受影响。" destructive confirmLabel="取消传输" onCancel={() => setPending(null)} onConfirm={() => { if (pending && !action.isPending) action.mutate(() => api.cancelTransfer(pending.id), { onSuccess: () => setPending(null) }) }} />
  </div>
}
