import { useState } from 'react'
import { Share2, Copy, Plus, Trash2, X } from 'lucide-react'
import api, { ShareEntry } from '../../services/api'
import { useAction, useResource } from '../../hooks/useResource'
import ResourceState from '../../components/ResourceState'
import ConfirmDialog from '../../components/ConfirmDialog'

export default function SharesPage() {
  const resource = useResource(['shares'], () => api.listShares())
  const files = useResource(['share-files'], () => api.listFiles())
  const action = useAction()
  const [creating, setCreating] = useState(false)
  const [fileId, setFileId] = useState('')
  const [expiry, setExpiry] = useState(86400)
  const [createdUrl, setCreatedUrl] = useState('')
  const [pending, setPending] = useState<ShareEntry | null>(null)
  const [message, setMessage] = useState('')
  const create = () => {
    if (!fileId || action.isPending) return
    action.mutate(() => api.createShare(fileId, expiry), { onSuccess: result => {
      const share = result as ShareEntry
      setCreatedUrl(`${window.location.origin}/s/${share.token}`)
      setCreating(false)
    } })
  }
  return <div className="page-shell">
    <ResourceState loading={resource.isPending} error={action.error || resource.error || files.error} retry={() => { action.reset(); void resource.refetch(); void files.refetch() }} />
    <div className="mb-6 flex justify-end"><button className="btn btn-primary" onClick={() => setCreating(true)}><Plus size={18} />创建链接</button></div>
    {createdUrl && <div className="card mb-4"><p className="mb-2 font-semibold">分享已创建，请保存链接</p><p className="app-muted mb-2 text-sm">出于安全原因，链接密钥只在创建时显示。</p><input className="input" readOnly value={createdUrl} aria-label="新分享链接" /><button className="btn btn-secondary mt-3" onClick={() => navigator.clipboard.writeText(createdUrl).then(() => setMessage('链接已复制')).catch(() => setMessage('请手动选择并复制链接'))}><Copy size={16} />复制链接</button></div>}
    {message && <p role="status" className="mb-3">{message}</p>}
    <div className="min-h-0 flex-1 space-y-3 overflow-auto">{(resource.data || []).map(share => <article key={share.id} className="card flex items-start gap-4"><Share2 className="text-primary-600" size={24} /><div className="min-w-0 flex-1"><h2 className="truncate font-semibold">{share.file_name}</h2><p className="app-muted mt-2 text-sm">有效期至 {new Date(share.expires_at).toLocaleString()}</p><p className="app-muted text-sm">已签发下载授权 {share.download_count} 次 · {share.status === 'active' && Date.parse(share.expires_at) > Date.now() ? '有效' : '已失效'}</p></div>{share.status === 'active' && <button className="icon-button text-red-600" onClick={() => setPending(share)} aria-label={`取消 ${share.file_name} 的分享`}><Trash2 size={18} /></button>}</article>)}
      {!resource.isPending && !resource.error && !resource.data?.length && <p className="app-muted py-12 text-center">暂无分享链接</p>}
    </div>
    {creating && <div className="modal-backdrop fixed inset-0 z-50 flex items-end justify-center p-3 sm:items-center"><div role="dialog" aria-modal="true" aria-labelledby="share-title" className="modal-panel app-surface w-full max-w-md rounded-2xl border p-5"><div className="mb-4 flex items-center justify-between"><h2 id="share-title" className="text-lg font-semibold">创建分享链接</h2><button className="icon-button" onClick={() => setCreating(false)} aria-label="关闭"><X size={18} /></button></div><label className="block">选择文件<select className="input mb-4 mt-2" value={fileId} onChange={event => setFileId(event.target.value)}><option value="">请选择有在线副本的文件</option>{(files.data || []).filter(file => file.available).map(file => <option key={file.id} value={file.id}>{file.name}</option>)}</select></label><label className="block">有效期<select className="input mt-2" value={expiry} onChange={event => setExpiry(Number(event.target.value))}><option value={86400}>24 小时</option><option value={604800}>7 天</option><option value={2592000}>30 天</option></select></label><p className="app-muted mt-3 text-sm">接收者需要能够连接到文件所在设备。</p><ResourceState error={action.error} /><div className="mt-5 flex justify-end gap-2"><button className="btn btn-secondary" onClick={() => setCreating(false)}>取消</button><button className="btn btn-primary" disabled={!fileId || action.isPending} onClick={create}>{action.isPending ? '创建中…' : '创建链接'}</button></div></div></div>}
    <ConfirmDialog open={!!pending} title={`取消“${pending?.file_name || ''}”的分享？`} description="将停止签发新下载授权，已签发授权将在到期后失效。" destructive onCancel={() => setPending(null)} onConfirm={() => { if (pending && !action.isPending) action.mutate(() => api.deleteShare(pending.id), { onSuccess: () => setPending(null) }) }} />
  </div>
}
