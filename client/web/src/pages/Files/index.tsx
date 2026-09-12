import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  Archive, Check, ChevronDown, ChevronRight, Download, File, FileText, Film,
  Folder, FolderOpen, FolderPlus, Grid2X2, Image, Info, Laptop, List, MoreHorizontal, Music,
  Send, Server, Share2, Smartphone, Trash2, Upload, WifiOff, X
} from 'lucide-react'
import { formatFileSize, getFileType } from '../../utils/helpers'
import ConfirmDialog from '../../components/ConfirmDialog'
import ResourceState from '../../components/ResourceState'
import api from '../../services/api'
import { submitDownload } from '../../services/agentData'
import { useResource, useAction } from '../../hooks/useResource'

type DeviceKind = 'desktop' | 'mobile' | 'server'
interface StorageDevice { id: string; name: string; kind: DeviceKind; online: boolean; ip: string }
interface FileItem {
  id: string
  name: string
  size: number
  type: string
  modified: string
  isFolder: boolean
  originDeviceId?: string
  replicaDeviceIds?: string[]
  available?: boolean
  version: number
}

const deviceIcons = { desktop: Laptop, mobile: Smartphone, server: Server }

function fileTypeLabel(type: string) {
  return ({ image: '图片', video: '视频', audio: '音频', document: '文档', other: '其他' } as const)[getFileType(type)]
}

const typeStyles = {
  image: 'bg-violet-50 text-violet-600 dark:bg-violet-950/40 dark:text-violet-300',
  video: 'bg-rose-50 text-rose-600 dark:bg-rose-950/40 dark:text-rose-300',
  audio: 'bg-amber-50 text-amber-600 dark:bg-amber-950/40 dark:text-amber-300',
  document: 'bg-blue-50 text-blue-600 dark:bg-blue-950/40 dark:text-blue-300',
  other: 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-300',
}

function FileMark({ file, large = false }: { file: FileItem; large?: boolean }) {
  if (file.isFolder) return <span className={`flex items-center justify-center rounded-2xl bg-primary-50 text-primary-600 dark:bg-primary-900/40 dark:text-primary-300 ${large ? 'h-16 w-16' : 'h-10 w-10'}`}><Folder size={large ? 31 : 21} fill="currentColor" strokeWidth={1.4} /></span>
  const kind = getFileType(file.type)
  const Icon = kind === 'image' ? Image : kind === 'video' ? Film : kind === 'audio' ? Music : kind === 'document' ? FileText : File
  return <span className={`flex items-center justify-center rounded-2xl ${typeStyles[kind]} ${large ? 'h-16 w-16' : 'h-10 w-10'}`}><Icon size={large ? 29 : 20} strokeWidth={1.7} /></span>
}

export default function FilesPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const [view, setView] = useState<'grid' | 'list'>(() => (localStorage.getItem('file-view') as 'grid' | 'list') || 'grid')
  const [path, setPath] = useState<{ id: string; name: string }[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [query, setQuery] = useState('')
  type FileKind = 'folder' | 'image' | 'document' | 'video' | 'audio' | 'other'
  const typeOptions: { value: FileKind; label: string }[] = [
    { value: 'folder', label: '文件夹' }, { value: 'image', label: '图片' },
    { value: 'document', label: '文档' }, { value: 'video', label: '视频' },
    { value: 'audio', label: '音频' }, { value: 'other', label: '其他' },
  ]
  const [typeFilters, setTypeFilters] = useState<FileKind[]>(() => {
    try { return JSON.parse(localStorage.getItem('file-type-filters') || '[]') as FileKind[] } catch { return [] }
  })
  const [timeSort, setTimeSort] = useState<'newest' | 'oldest'>('newest')
  const deviceFilter = searchParams.get('device') || 'all'
  const [folderDialog, setFolderDialog] = useState(false)
  const [editor, setEditor] = useState<{ kind: 'rename' | 'move'; file: FileItem } | null>(null)
  const [editValue, setEditValue] = useState('')
  const [folderName, setFolderName] = useState('')
  const [notice, setNotice] = useState('')
  const [detailFileId, setDetailFileId] = useState<string | null>(null)
  const [targetDeviceId, setTargetDeviceId] = useState('')
  const [contextMenu, setContextMenu] = useState<{ fileId: string; x: number; y: number } | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)
  const foldersQuery = useResource(['folders'], () => api.listFolders())
  const devicesQuery = useResource(['devices'], () => api.listDevices(), 15000)
  const folders = foldersQuery.data || []
  const parentId = path[path.length - 1]?.id || folders.find(folder => !folder.parent_id)?.id
  const filesQuery = useResource(['files', parentId, query, timeSort], () => api.listFiles(parentId, query, timeSort), 15000)
  const action = useAction()
  const storageDevices: StorageDevice[] = (devicesQuery.data || []).map(device => ({
    id: device.id, name: device.name, kind: device.platform === 'android' ? 'mobile' : device.platform === 'server' ? 'server' : 'desktop',
    online: device.status === 'active' && !!device.last_seen_at && Date.now() - Date.parse(device.last_seen_at) < 120000, ip: device.platform,
  }))
  const files: FileItem[] = [
    ...folders.filter(folder => folder.parent_id === parentId).map(folder => ({ id: folder.id, name: folder.name, size: 0, type: '', modified: new Date(folder.updated_at).toLocaleString(), isFolder: true, version: folder.version })),
    ...(filesQuery.data || []).map(file => ({ id: file.id, name: file.name, size: file.size, type: file.mime, modified: new Date(file.updated_at).toLocaleString(), isFolder: false, originDeviceId: file.origin_device_id, replicaDeviceIds: (file.replicas || []).filter(replica => replica.state === 'ready').map(replica => replica.device_id), available: file.available, version: file.version })),
  ]
  const replicasFor = (file: FileItem) => storageDevices.filter(device => file.replicaDeviceIds?.includes(device.id))
  const isFileAvailable = (file: FileItem) => file.isFolder || file.available === true


  const visibleFiles = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    const filtered = files.filter(file => {
      const kind = file.isFolder ? 'folder' : getFileType(file.type)
      const matchesDevice = deviceFilter === 'all' || (!file.isFolder && file.replicaDeviceIds?.includes(deviceFilter))
      return file.name.toLowerCase().includes(normalizedQuery) && (typeFilters.length === 0 || typeFilters.includes(kind as FileKind)) && matchesDevice
    })
    return filtered
  }, [deviceFilter, files, query, timeSort, typeFilters])
  const detailFile = files.find(file => file.id === detailFileId)
  const selectedFiles = files.filter(file => selected.includes(file.id))
  const selectedDownloadable = selectedFiles.length > 0 && selectedFiles.every(file => !file.isFolder && isFileAvailable(file))
  const detailReplicas = detailFile ? replicasFor(detailFile) : []
  const availableReplicaCount = detailReplicas.filter(device => device.online).length
  const targetDevices = detailFile ? storageDevices.filter(device => !detailFile.replicaDeviceIds?.includes(device.id)) : []

  useEffect(() => {
    const onSearch = (event: Event) => setQuery((event as CustomEvent<string>).detail || '')
    window.addEventListener('share-disk-search', onSearch)
    return () => {
      window.removeEventListener('share-disk-search', onSearch)
    }
  }, [])

  useEffect(() => {
    if (!contextMenu) return
    const close = () => setContextMenu(null)
    const onKeyDown = (event: KeyboardEvent) => event.key === 'Escape' && close()
    window.addEventListener('click', close)
    window.addEventListener('keydown', onKeyDown)
    return () => { window.removeEventListener('click', close); window.removeEventListener('keydown', onKeyDown) }
  }, [contextMenu])

  useEffect(() => {
    if (!detailFileId) return
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') { setDetailFileId(null); setTargetDeviceId('') }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [detailFileId])

  const showNotice = (message: string) => { setNotice(message); window.setTimeout(() => setNotice(''), 2200) }
  const changeView = (next: 'grid' | 'list') => { setView(next); localStorage.setItem('file-view', next) }
  const toggleTypeFilter = (value: FileKind) => {
    setTypeFilters(current => {
      const next = current.includes(value) ? current.filter(item => item !== value) : [...current, value]
      localStorage.setItem('file-type-filters', JSON.stringify(next))
      return next
    })
  }
  const toggleSelected = (id: string) => setSelected(current => current.includes(id) ? current.filter(item => item !== id) : [...current, id])
  const openFile = (file: FileItem) => {
    if (file.isFolder) { setPath(current => [...current, { id: file.id, name: file.name }]); setSelected([]); return }
    if (window.matchMedia('(max-width: 640px)').matches) { setDetailFileId(file.id); return }
    if (isFileAvailable(file)) toggleSelected(file.id)
  }
  const openContextMenu = (event: React.MouseEvent, file: FileItem) => {
    event.preventDefault()
    setContextMenu({ fileId: file.id, x: Math.min(event.clientX, window.innerWidth - 196), y: Math.min(event.clientY, window.innerHeight - 168) })
  }
  const downloadFile = (file: FileItem) => {
    if (!isFileAvailable(file) || file.isFolder) return
    action.mutate(async () => { const result = await api.downloadTicket(file.id); submitDownload(result.url, result.ticket) })
  }
  const sendToDevice = () => {
    if (!detailFile || !targetDeviceId || !isFileAvailable(detailFile) || action.isPending) return
    action.mutate(() => api.replicateFile(detailFile.id, targetDeviceId), { onSuccess: () => { setTargetDeviceId(''); showNotice('副本任务已创建，完成状态请查看传输任务。') } })
  }
  const deleteSelectedFiles = () => {
    if (action.isPending) return
    action.mutate(async () => {
      const files = selectedFiles.filter(file => !file.isFolder)
      if (files.length) await api.fileAction(files.map(file => file.id), 'trash', undefined, Object.fromEntries(files.map(file => [file.id, file.version])))
      for (const folder of selectedFiles.filter(file => file.isFolder)) await api.deleteFolder(folder.id)
    }, { onSuccess: () => { setSelected([]); setConfirmDelete(false); showNotice('所选文件已移入回收站，空文件夹已删除') } })
  }
  const createFolder = () => {
    const name = folderName.trim()
    if (!name || action.isPending) return
    action.mutate(() => api.createFolder(name, parentId), { onSuccess: () => { setFolderName(''); setFolderDialog(false); showNotice('文件夹已创建') } })
  }
  const startEdit = (kind: 'rename' | 'move', file: FileItem) => {
    action.reset(); setContextMenu(null); setDetailFileId(null)
    setEditor({ kind, file }); setEditValue(kind === 'rename' ? file.name : parentId || '')
  }
  const saveEdit = () => {
    if (!editor || !editValue.trim() || action.isPending) return
    action.mutate(async () => {
      if (editor.kind === 'move') await api.moveFiles([editor.file.id], editValue, { [editor.file.id]: editor.file.version })
      else if (editor.file.isFolder) await api.renameFolder(editor.file.id, editValue.trim())
      else await api.fileAction([editor.file.id], 'rename', editValue.trim(), { [editor.file.id]: editor.file.version })
    }, { onSuccess: () => { setEditor(null); setSelected([]); showNotice('更改已保存') } })
  }
  const importFiles = (_list: FileList | null) => showNotice('浏览器上传尚未启用，请通过原生客户端上传。')

  return (
    <div className="page-shell">
      <ResourceState loading={filesQuery.isPending || foldersQuery.isPending} error={action.error || filesQuery.error || foldersQuery.error || devicesQuery.error} retry={() => { action.reset(); void filesQuery.refetch(); void foldersQuery.refetch(); void devicesQuery.refetch() }} />
      <input aria-label="搜索文件" className="input mb-3" value={query} onChange={event => setQuery(event.target.value)} placeholder="搜索文件名称" />
      <div className="mb-5 flex min-h-11 flex-wrap items-center justify-between gap-4">
        <div className="lg:hidden"><h1 className="page-title">全部文件</h1></div>
        <div className="hidden min-w-0 flex-1 items-center gap-1 overflow-x-auto whitespace-nowrap text-sm lg:flex">
          {path.length > 0 && <button onClick={() => setPath([])} className="icon-button h-11 w-11 shrink-0" aria-label="返回文件根目录" title="根目录"><FolderOpen size={18} /></button>}
          {path.map((folder, index) => <span key={`${folder.id}-${index}`} className="flex items-center gap-1"><ChevronRight size={15} className={`app-muted ${index === 0 ? 'hidden' : ''}`} /><button onClick={() => setPath(current => current.slice(0, index + 1))} className={`h-11 rounded-xl px-3 font-medium ${index === path.length - 1 ? 'bg-[var(--surface-soft)]' : 'app-muted hover:bg-[var(--surface-soft)] hover:text-primary-600'}`}>{folder.name}</button></span>)}
        </div>
        <div className="flex gap-2">
          <button className="btn btn-secondary px-3 sm:px-4" onClick={() => setFolderDialog(true)}><FolderPlus size={18} /><span className="hidden sm:inline">新建文件夹</span></button>
          <button className="btn btn-primary px-3 sm:px-4" onClick={() => fileInput.current?.click()}><Upload size={18} /><span className="hidden sm:inline">上传文件</span></button>
          <input ref={fileInput} className="hidden" type="file" multiple onChange={e => importFiles(e.target.files)} />
        </div>
      </div>

      {path.length > 0 && <div className="mb-4 flex min-h-10 items-center gap-1 overflow-x-auto whitespace-nowrap text-sm lg:hidden">
        <button onClick={() => setPath([])} className="app-muted rounded-lg px-2 py-1.5 font-medium hover:bg-[var(--surface)] hover:text-primary-600">我的空间</button>
        {path.map((folder, index) => <span key={`${folder.id}-${index}`} className="flex items-center gap-1"><ChevronRight size={15} className="app-muted" /><button onClick={() => setPath(current => current.slice(0, index + 1))} className={`rounded-lg px-2 py-1.5 font-medium ${index === path.length - 1 ? '' : 'app-muted hover:text-primary-600'}`}>{folder.name}</button></span>)}
      </div>}

      <div className="app-surface mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-2.5">
        <details className="group relative min-w-[170px] flex-1 sm:flex-none">
          <summary className="flex h-11 cursor-pointer list-none items-center justify-between rounded-xl border border-[var(--border)] bg-[var(--surface-soft)] px-4 text-sm font-medium outline-none transition-colors hover:border-primary-300 focus:ring-2 focus:ring-primary-500/15">
            <span>{typeFilters.length === 0 ? '全部类型' : `已选 ${typeFilters.length} 类`}</span><ChevronDown size={16} className="app-muted transition-transform group-open:rotate-180" />
          </summary>
          <div className="app-surface absolute left-0 top-12 z-30 grid min-w-full grid-cols-2 gap-1 rounded-xl border p-2 shadow-xl sm:w-56">
            {typeOptions.map(option => <label key={option.value} className="flex min-h-10 cursor-pointer items-center gap-2 rounded-lg px-2.5 text-sm hover:bg-[var(--surface-soft)]"><input type="checkbox" checked={typeFilters.includes(option.value)} onChange={() => toggleTypeFilter(option.value)} className="h-4 w-4 accent-primary-600" />{option.label}</label>)}
            {typeFilters.length > 0 && <button type="button" onClick={() => { setTypeFilters([]); localStorage.removeItem('file-type-filters') }} className="col-span-2 min-h-9 rounded-lg text-sm font-medium text-primary-700 hover:bg-[var(--surface-soft)] dark:text-primary-300">清除筛选</button>}
          </div>
        </details>
        <label className="relative min-w-[170px] flex-1 sm:flex-none">
          <span className="sr-only">设备筛选</span>
          <select value={deviceFilter} onChange={event => { const value = event.target.value; setSearchParams(value === 'all' ? {} : { device: value }) }} className="h-11 w-full appearance-none rounded-xl border border-[var(--border)] bg-[var(--surface-soft)] pl-4 pr-10 text-sm font-medium outline-none transition-colors hover:border-primary-300 focus:border-primary-500 focus:ring-2 focus:ring-primary-500/15">
            <option value="all">全部设备</option>{storageDevices.map(device => <option key={device.id} value={device.id}>{device.name}</option>)}
          </select>
          <ChevronDown size={16} className="app-muted pointer-events-none absolute right-3 top-1/2 -translate-y-1/2" />
        </label>
        <label className="relative min-w-[150px] flex-1 sm:flex-none">
          <span className="sr-only">时间排序</span>
          <select value={timeSort} onChange={event => setTimeSort(event.target.value as typeof timeSort)} className="h-11 w-full appearance-none rounded-xl border border-[var(--border)] bg-[var(--surface-soft)] pl-4 pr-10 text-sm font-medium outline-none transition-colors hover:border-primary-300 focus:border-primary-500 focus:ring-2 focus:ring-primary-500/15">
            <option value="newest">最新优先</option><option value="oldest">最早优先</option>
          </select>
          <ChevronDown size={16} className="app-muted pointer-events-none absolute right-3 top-1/2 -translate-y-1/2" />
        </label>
        <div className="ml-auto flex rounded-xl bg-[var(--surface-soft)] p-1" role="group" aria-label="视图模式">
          <button onClick={() => changeView('grid')} className={`flex h-8 w-9 items-center justify-center rounded-lg ${view === 'grid' ? 'app-surface shadow-sm' : 'app-muted'}`} aria-label="网格视图"><Grid2X2 size={17} /></button>
          <button onClick={() => changeView('list')} className={`flex h-8 w-9 items-center justify-center rounded-lg ${view === 'list' ? 'app-surface shadow-sm' : 'app-muted'}`} aria-label="列表视图"><List size={18} /></button>
        </div>
      </div>

      {selected.length > 0 && (
        <div className="mb-4 flex min-h-12 flex-wrap items-center gap-1 rounded-xl bg-primary-50 px-3 text-sm text-primary-800 dark:bg-primary-900/40 dark:text-primary-200 sm:fixed sm:bottom-6 sm:left-1/2 sm:z-40 sm:mb-0 sm:-translate-x-1/2 sm:border sm:border-primary-200 sm:bg-[var(--surface)] sm:px-4 sm:shadow-xl dark:sm:border-primary-800 dark:sm:bg-[var(--surface)]">
          <span className="mr-2 font-semibold">已选择 {selected.length} 项</span>
          <button disabled={!selectedDownloadable} onClick={() => selectedFiles.forEach(downloadFile)} className="btn btn-ghost min-h-9 px-2 text-primary-700 dark:text-primary-200" title={selectedDownloadable ? '下载所选文件' : '所选文件当前没有在线副本'}><Download size={16} />下载</button>
          <button disabled={action.isPending || selectedFiles.some(file => file.isFolder || !file.available)} onClick={() => action.mutate(async () => { const links = []; for (const file of selectedFiles) { const share = await api.createShare(file.id, 86400); links.push(`${window.location.origin}/s/${share.token}`) }; setNotice(links.join("\n")) })} className="btn btn-ghost min-h-9 px-2 text-primary-700 dark:text-primary-200"><Share2 size={16} />分享</button>
          <button className="btn btn-ghost min-h-9 px-2 text-red-600" onClick={() => setConfirmDelete(true)}><Trash2 size={16} />删除</button>
          <button className="icon-button ml-auto h-9 w-9" onClick={() => setSelected([])} aria-label="取消选择"><X size={17} /></button>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-auto pb-2">
        {visibleFiles.length === 0 ? (
          <div className="flex h-full min-h-64 flex-col items-center justify-center text-center"><span className="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-[var(--surface)]"><Archive className="app-muted" size={28} /></span><h2 className="font-semibold">没有找到文件</h2><p className="app-muted mt-1 text-sm">换个关键词，或上传一个新文件</p></div>
        ) : view === 'grid' ? (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-5 2xl:grid-cols-6">
            {visibleFiles.map(file => {
              const active = selected.includes(file.id)
              const available = isFileAvailable(file)
              return <article key={file.id} onContextMenu={event => openContextMenu(event, file)} onDoubleClick={() => file.isFolder && openFile(file)} className={`card group relative cursor-default p-3 transition-all ${available ? 'hover:-translate-y-0.5 hover:shadow-md' : 'opacity-55 grayscale'} ${active ? 'border-primary-500 ring-2 ring-primary-500/15' : ''}`}>
                {<button onClick={() => toggleSelected(file.id)} className={`absolute right-3 top-3 z-10 flex h-6 w-6 items-center justify-center rounded-lg border transition-opacity ${active ? 'border-primary-600 bg-primary-600 text-white' : 'border-[var(--border)] bg-[var(--surface)] opacity-100 sm:opacity-0 sm:group-hover:opacity-100'}`} aria-label={`选择 ${file.name}`}>{active && <Check size={14} />}</button>}
                {<button onClick={() => file.isFolder ? startEdit('rename', file) : setDetailFileId(file.id)} className="icon-button absolute left-2 top-2 z-10 h-8 w-8 bg-[var(--surface)] opacity-100 shadow-sm sm:opacity-0 sm:group-hover:opacity-100" aria-label={`查看 ${file.name} 属性`}><MoreHorizontal size={17} /></button>}
                <button onClick={() => openFile(file)} aria-disabled={!available} className={`flex w-full flex-col items-start rounded-xl text-left ${!available ? 'cursor-not-allowed' : ''}`}><div className="mb-5 flex h-24 w-full items-center justify-center rounded-xl bg-[var(--surface-soft)]"><FileMark file={file} large /></div><span className="flex w-full min-w-0 items-center gap-2"><span className="min-w-0 flex-1 truncate text-sm font-semibold">{file.name}</span>{!file.isFolder && <span className={`shrink-0 rounded-full px-2 py-0.5 text-[11px] font-medium ${available ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300' : 'bg-gray-100 text-gray-500 dark:bg-gray-800'}`}>{available ? `${replicasFor(file).filter(device => device.online).length} 台在线` : '不可用'}</span>}</span><span className="app-muted mt-1 text-xs">{file.isFolder ? file.modified : `${formatFileSize(file.size)} · ${file.modified}`}</span></button>
              </article>
            })}
          </div>
        ) : (
          <div className="app-surface overflow-hidden rounded-2xl border">
            <div className="app-muted hidden grid-cols-[minmax(240px,1fr)_120px_150px_44px] border-b border-[var(--border)] px-4 py-3 text-xs font-medium sm:grid"><span>名称</span><span>大小</span><span>修改时间</span><span /></div>
            {visibleFiles.map(file => {
              const available = isFileAvailable(file)
              return <div key={file.id} onContextMenu={event => openContextMenu(event, file)} className={`grid grid-cols-[minmax(0,1fr)_44px] items-center gap-2 border-b border-[var(--border)] px-3 py-2.5 last:border-0 sm:grid-cols-[minmax(240px,1fr)_120px_150px_44px] ${available ? 'hover:bg-[var(--surface-soft)]' : 'bg-gray-50 opacity-55 grayscale dark:bg-gray-900/20'} ${selected.includes(file.id) ? 'bg-primary-50 dark:bg-primary-900/30' : ''}`}>
                <button onClick={() => openFile(file)} aria-disabled={!available} className={`flex min-w-0 items-center gap-3 text-left ${!available ? 'cursor-not-allowed' : ''}`}><FileMark file={file} /><span className="min-w-0 flex-1 truncate text-sm font-medium">{file.name}</span>{!file.isFolder && <span className={`rounded-full px-2 py-0.5 text-[11px] font-medium sm:hidden ${available ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300' : 'bg-gray-200 text-gray-600 dark:bg-gray-800 dark:text-gray-400'}`}>{available ? '在线' : '不可用'}</span>}</button>
                <span className="app-muted hidden text-sm sm:block">{file.isFolder ? '—' : formatFileSize(file.size)}</span><span className="app-muted hidden text-sm sm:block">{file.modified}</span>{<button onClick={() => file.isFolder ? startEdit('rename', file) : setDetailFileId(file.id)} className="icon-button" aria-label={`查看 ${file.name} 属性`}><MoreHorizontal size={18} /></button>}
              </div>
            })}
          </div>
        )}
      </div>

      {contextMenu && (() => {
        const file = files.find(item => item.id === contextMenu.fileId)
        if (!file) return null
        const available = isFileAvailable(file)
        return <div role="menu" className="app-surface fixed z-50 w-48 rounded-xl border p-1.5 shadow-xl" style={{ left: contextMenu.x, top: contextMenu.y }} onClick={event => event.stopPropagation()}>
          <button role="menuitem" disabled={file.isFolder || !available} onClick={() => { downloadFile(file); setContextMenu(null) }} className="flex min-h-10 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium hover:bg-[var(--surface-soft)] disabled:cursor-not-allowed disabled:opacity-40"><Download size={17} />下载</button>
          <button role="menuitem" onClick={() => { setDetailFileId(file.id); setContextMenu(null) }} className="flex min-h-10 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium hover:bg-[var(--surface-soft)]"><Info size={17} />属性</button>
          <button role="menuitem" onClick={() => startEdit('rename', file)} className="flex min-h-10 w-full items-center rounded-lg px-3 text-sm hover:bg-[var(--surface-soft)]">重命名</button>
          {!file.isFolder && <button role="menuitem" onClick={() => startEdit('move', file)} className="flex min-h-10 w-full items-center rounded-lg px-3 text-sm hover:bg-[var(--surface-soft)]">移动到…</button>}
          <div className="my-1 border-t border-[var(--border)]" />
          <button role="menuitem" onClick={() => { setSelected([file.id]); setConfirmDelete(true); setContextMenu(null) }} className="flex min-h-10 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-red-950/30"><Trash2 size={17} />删除</button>
        </div>
      })()}

      {detailFile && !detailFile.isFolder && (() => {
        const origin = storageDevices.find(device => device.id === detailFile.originDeviceId)
        const available = isFileAvailable(detailFile)
        return <div className="modal-backdrop fixed inset-0 z-50 flex items-end justify-end" onMouseDown={() => { setDetailFileId(null); setTargetDeviceId('') }}>
          <aside role="dialog" aria-modal="true" aria-labelledby="file-detail-title" onMouseDown={event => event.stopPropagation()} className="modal-panel app-surface flex max-h-[92dvh] w-full flex-col rounded-t-[24px] border shadow-2xl sm:h-full sm:max-h-none sm:max-w-md sm:rounded-none sm:border-y-0 sm:border-r-0">
            <div className="flex shrink-0 items-start gap-3 border-b border-[var(--border)] p-5">
              <FileMark file={detailFile} />
              <div className="min-w-0 flex-1"><h2 id="file-detail-title" className="truncate text-lg font-semibold">{detailFile.name}</h2><p className="app-muted mt-0.5 text-sm">{formatFileSize(detailFile.size)} · {fileTypeLabel(detailFile.type)}</p></div>
              <button className="icon-button" onClick={() => { setDetailFileId(null); setTargetDeviceId('') }} aria-label="关闭属性"><X size={20} /></button>
            </div>

            <div className="min-h-0 flex-1 overflow-auto p-5">
              <div className={`mb-6 flex items-start gap-3 rounded-2xl p-4 ${available ? 'bg-primary-50 text-primary-900 dark:bg-primary-900/40 dark:text-primary-100' : 'bg-gray-100 text-gray-700 dark:bg-gray-800 dark:text-gray-300'}`}>
                {available ? <Download className="mt-0.5 shrink-0" size={19} /> : <WifiOff className="mt-0.5 shrink-0" size={19} />}
                <p className="text-sm font-semibold">{available ? `${availableReplicaCount} 台设备在线` : '文件当前不可用'}</p>
              </div>

              <div className="mb-5 flex flex-wrap gap-2"><button className="btn btn-secondary" onClick={() => startEdit('rename', detailFile)}>重命名</button><button className="btn btn-secondary" onClick={() => startEdit('move', detailFile)}>移动到…</button><button className="btn btn-danger" onClick={() => { setSelected([detailFile.id]); setDetailFileId(null); setConfirmDelete(true) }}>删除</button></div>
              <section className="mb-7">
                <h3 className="mb-3 text-sm font-semibold">文件信息</h3>
                <dl className="grid grid-cols-[88px_1fr] gap-y-2.5 text-sm"><dt className="app-muted">修改时间</dt><dd>{detailFile.modified}</dd><dt className="app-muted">上传来源</dt><dd className="flex items-center gap-2">{origin?.name || '未知设备'}{origin && <span className={`h-2 w-2 rounded-full ${origin.online ? 'bg-primary-500' : 'bg-gray-400'}`} aria-label={origin.online ? '在线' : '离线'} />}</dd></dl>
              </section>

              <section className="mb-7">
                <div className="mb-3 flex items-baseline justify-between"><h3 className="text-sm font-semibold">已有副本</h3><span className="app-muted text-xs">{detailReplicas.length} 台设备</span></div>
                <div className="space-y-2">
                  {detailReplicas.map(device => {
                    const Icon = deviceIcons[device.kind]
                    return <div key={device.id} className="flex items-center gap-3 rounded-xl border border-[var(--border)] p-3"><span className="flex h-10 w-10 items-center justify-center rounded-xl bg-[var(--surface-soft)]"><Icon size={19} /></span><div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{device.name}</p><p className="app-muted mt-0.5 text-xs">{device.ip}{device.id === detailFile.originDeviceId ? ' · 上传设备' : ''}</p></div><span className={`flex items-center gap-1.5 text-xs font-medium ${device.online ? 'text-primary-700 dark:text-primary-300' : 'app-muted'}`}><span className={`h-2 w-2 rounded-full ${device.online ? 'bg-primary-500' : 'bg-gray-400'}`} />{device.online ? '在线' : '离线'}</span></div>
                  })}
                </div>
              </section>

              <section>
                <h3 className="text-sm font-semibold">发送到设备</h3>
                <p className="app-muted mb-3 mt-1 text-xs leading-5">选择后，目标设备会主动下载并保留一份副本。</p>
                {targetDevices.length > 0 ? <div className="space-y-2">{targetDevices.map(device => {
                  const Icon = deviceIcons[device.kind]
                  const enabled = available && device.online
                  return <label key={device.id} className={`flex items-center gap-3 rounded-xl border p-3 transition-colors ${targetDeviceId === device.id ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/30' : 'border-[var(--border)]'} ${enabled ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'}`}><input type="radio" name="target-device" value={device.id} checked={targetDeviceId === device.id} onChange={() => setTargetDeviceId(device.id)} disabled={!enabled} className="sr-only" /><span className="flex h-10 w-10 items-center justify-center rounded-xl bg-[var(--surface-soft)]"><Icon size={19} /></span><span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{device.name}</span><span className="app-muted mt-0.5 block text-xs">{device.ip}</span></span><span className={`text-xs font-medium ${device.online ? 'text-primary-700 dark:text-primary-300' : 'app-muted'}`}>{device.online ? '在线' : '离线'}</span>{targetDeviceId === device.id && <Check size={17} className="text-primary-600" />}</label>
                })}</div> : <div className="rounded-xl bg-[var(--surface-soft)] p-4 text-center text-sm"><p className="font-medium">所有设备都已有该文件</p></div>}
              </section>
            </div>

            <div className="safe-bottom app-surface grid shrink-0 grid-cols-2 gap-2 border-t border-[var(--border)] p-4">
              <button disabled={!available} onClick={() => downloadFile(detailFile)} className="btn btn-secondary"><Download size={18} />{available ? '下载到本机' : '暂不可下载'}</button>
              <button disabled={!targetDeviceId || !available} onClick={sendToDevice} className="btn btn-primary"><Send size={18} />发送到设备</button>
            </div>
          </aside>
        </div>
      })()}

      {editor && <div className="modal-backdrop fixed inset-0 z-50 flex items-end justify-center p-3 sm:items-center"><form role="dialog" aria-modal="true" aria-labelledby="edit-title" onSubmit={event => { event.preventDefault(); saveEdit() }} className="modal-panel app-surface w-full max-w-md rounded-2xl border p-5"><h2 id="edit-title" className="mb-4 text-lg font-semibold">{editor.kind === 'rename' ? '重命名' : '移动文件'}：{editor.file.name}</h2><ResourceState error={action.error} />{editor.kind === 'rename' ? <label className="block text-sm">新名称<input autoFocus className="input mt-2" value={editValue} onChange={event => setEditValue(event.target.value)} /></label> : <label className="block text-sm">目标文件夹<select className="input mt-2" value={editValue} onChange={event => setEditValue(event.target.value)}>{folders.map(folder => <option key={folder.id} value={folder.id}>{folder.parent_id ? folder.name : '我的空间（根目录）'}</option>)}</select></label>}<div className="mt-5 flex justify-end gap-2"><button type="button" className="btn btn-secondary" onClick={() => setEditor(null)}>取消</button><button className="btn btn-primary" disabled={!editValue.trim() || action.isPending}>{action.isPending ? '保存中…' : '保存'}</button></div></form></div>}
      {folderDialog && <div className="modal-backdrop fixed inset-0 z-50 flex items-end justify-center p-3 sm:items-center" onMouseDown={() => setFolderDialog(false)}><div role="dialog" aria-modal="true" aria-labelledby="folder-title" onMouseDown={e => e.stopPropagation()} className="modal-panel app-surface w-full max-w-md rounded-2xl border p-5 shadow-2xl"><div className="mb-5 flex items-center justify-between"><div><h2 id="folder-title" className="text-lg font-semibold">新建文件夹</h2><p className="app-muted mt-1 text-sm">文件夹将创建在当前位置</p></div><button className="icon-button" onClick={() => setFolderDialog(false)} aria-label="关闭"><X size={19} /></button></div><label className="mb-5 block text-sm font-medium">文件夹名称<input autoFocus value={folderName} onChange={e => setFolderName(e.target.value)} onKeyDown={e => e.key === 'Enter' && createFolder()} className="input mt-2" placeholder="例如：项目资料" /></label><div className="flex justify-end gap-2"><button className="btn btn-secondary" onClick={() => setFolderDialog(false)}>取消</button><button className="btn btn-primary" disabled={!folderName.trim()} onClick={createFolder}>创建</button></div></div></div>}
      <ConfirmDialog open={confirmDelete} title={`删除 ${selected.length} 个项目？`} description="文件可从回收站恢复；空文件夹将直接删除，非空文件夹不会删除。" confirmLabel="移至回收站" destructive onCancel={() => setConfirmDelete(false)} onConfirm={deleteSelectedFiles} />
      {notice && <div role="status" className="fixed bottom-24 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-xl bg-gray-900 px-4 py-3 text-sm font-medium text-white shadow-xl sm:bottom-6"><Check size={17} className="text-primary-300" />{notice}</div>}
    </div>
  )
}
