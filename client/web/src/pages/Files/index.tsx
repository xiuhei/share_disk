import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import {
  Archive, Check, ChevronDown, ChevronRight, Download, File, FileText, Film,
  Folder, FolderOpen, FolderPlus, Grid2X2, Image, Info, Laptop, List, MoreHorizontal, Music,
  Send, Server, Share2, Smartphone, Trash2, Upload, WifiOff, X
} from 'lucide-react'
import { formatFileSize, getFileType } from '../../utils/helpers'
import ConfirmDialog from '../../components/ConfirmDialog'

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
}

const storageDevices: StorageDevice[] = [
  { id: 'ubuntu', name: 'Ubuntu 工作站', kind: 'desktop', online: true, ip: '192.168.1.100' },
  { id: 'android', name: '我的手机', kind: 'mobile', online: true, ip: '192.168.1.101' },
  { id: 'nas', name: '客厅 NAS', kind: 'server', online: true, ip: '192.168.1.106' },
  { id: 'work', name: '办公室电脑', kind: 'desktop', online: true, ip: '203.0.113.18' },
  { id: 'macbook', name: 'MacBook Pro', kind: 'desktop', online: false, ip: '192.168.1.108' },
]

const initialFiles: FileItem[] = [
  { id: '1', name: '团队资料', size: 0, type: '', modified: '今天 09:42', isFolder: true },
  { id: '2', name: '产品设计', size: 0, type: '', modified: '昨天 18:20', isFolder: true },
  { id: '3', name: '照片备份', size: 0, type: '', modified: '9 月 4 日', isFolder: true },
  { id: '4', name: '季度复盘.pdf', size: 2548000, type: 'application/pdf', modified: '今天 08:15', isFolder: false, originDeviceId: 'ubuntu', replicaDeviceIds: ['ubuntu', 'android', 'work'] },
  { id: '5', name: '需求清单.docx', size: 156000, type: 'application/docx', modified: '昨天 16:34', isFolder: false, originDeviceId: 'android', replicaDeviceIds: ['android', 'nas'] },
  { id: '6', name: '首页方案.png', size: 3200000, type: 'image/png', modified: '9 月 4 日', isFolder: false, originDeviceId: 'macbook', replicaDeviceIds: ['macbook'] },
  { id: '7', name: '发布演示.mp4', size: 156000000, type: 'video/mp4', modified: '9 月 3 日', isFolder: false, originDeviceId: 'macbook', replicaDeviceIds: ['macbook', 'ubuntu'] },
  { id: '8', name: '访谈录音.mp3', size: 8500000, type: 'audio/mp3', modified: '9 月 2 日', isFolder: false, originDeviceId: 'android', replicaDeviceIds: ['android'] },
]

const deviceIcons = { desktop: Laptop, mobile: Smartphone, server: Server }

function replicasFor(file: FileItem) {
  return storageDevices.filter(device => file.replicaDeviceIds?.includes(device.id))
}

function isFileAvailable(file: FileItem) {
  return file.isFolder || replicasFor(file).some(device => device.online)
}

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
  const [files, setFiles] = useState(initialFiles)
  const [view, setView] = useState<'grid' | 'list'>(() => (localStorage.getItem('file-view') as 'grid' | 'list') || 'grid')
  const [path, setPath] = useState<string[]>([])
  const [selected, setSelected] = useState<string[]>([])
  const [query, setQuery] = useState('')
  const [typeFilter, setTypeFilter] = useState<'all' | 'folder' | 'image' | 'document' | 'video' | 'audio' | 'other'>('all')
  const [timeSort, setTimeSort] = useState<'newest' | 'oldest'>('newest')
  const deviceFilter = searchParams.get('device') || 'all'
  const [folderDialog, setFolderDialog] = useState(false)
  const [folderName, setFolderName] = useState('')
  const [notice, setNotice] = useState('')
  const [detailFileId, setDetailFileId] = useState<string | null>(null)
  const [targetDeviceId, setTargetDeviceId] = useState('')
  const [contextMenu, setContextMenu] = useState<{ fileId: string; x: number; y: number } | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)

  const visibleFiles = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase()
    const filtered = files.filter(file => {
      const kind = file.isFolder ? 'folder' : getFileType(file.type)
      const matchesDevice = deviceFilter === 'all' || (!file.isFolder && file.replicaDeviceIds?.includes(deviceFilter))
      return file.name.toLowerCase().includes(normalizedQuery) && (typeFilter === 'all' || kind === typeFilter) && matchesDevice
    })
    return timeSort === 'newest' ? filtered : [...filtered].reverse()
  }, [deviceFilter, files, query, timeSort, typeFilter])
  const detailFile = files.find(file => file.id === detailFileId)
  const selectedFiles = files.filter(file => selected.includes(file.id))
  const selectedDownloadable = selectedFiles.length > 0 && selectedFiles.every(isFileAvailable)
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
  const toggleSelected = (id: string) => setSelected(current => current.includes(id) ? current.filter(item => item !== id) : [...current, id])
  const openFile = (file: FileItem) => {
    if (file.isFolder) { setPath(current => [...current, file.name]); setSelected([]); return }
    if (window.matchMedia('(max-width: 640px)').matches) { setDetailFileId(file.id); return }
    if (isFileAvailable(file)) toggleSelected(file.id)
  }
  const openContextMenu = (event: React.MouseEvent, file: FileItem) => {
    if (file.isFolder) return
    event.preventDefault()
    setContextMenu({ fileId: file.id, x: Math.min(event.clientX, window.innerWidth - 196), y: Math.min(event.clientY, window.innerHeight - 168) })
  }
  const downloadFile = (file: FileItem) => {
    if (!isFileAvailable(file)) return
    showNotice(`正在从在线设备获取“${file.name}”`)
  }
  const sendToDevice = () => {
    if (!detailFile || !targetDeviceId || !isFileAvailable(detailFile)) return
    const target = storageDevices.find(device => device.id === targetDeviceId)
    if (!target?.online) return
    setFiles(current => current.map(file => file.id === detailFile.id ? { ...file, replicaDeviceIds: [...(file.replicaDeviceIds || []), target.id] } : file))
    setTargetDeviceId('')
    showNotice(`已通知“${target.name}”下载该文件`)
  }
  const deleteSelectedFiles = () => {
    setFiles(items => items.filter(item => !selected.includes(item.id)))
    setSelected([])
    setConfirmDelete(false)
    showNotice('已移至回收站')
  }
  const createFolder = () => {
    const name = folderName.trim()
    if (!name) return
    setFiles(current => [{ id: crypto.randomUUID(), name, size: 0, type: '', modified: '刚刚', isFolder: true }, ...current])
    setFolderName(''); setFolderDialog(false); showNotice(`已创建“${name}”`)
  }
  const importFiles = (list: FileList | null) => {
    if (!list?.length) return
    const currentDeviceId = window.matchMedia('(max-width: 1024px)').matches ? 'android' : 'ubuntu'
    const additions = Array.from(list).map(item => ({ id: crypto.randomUUID(), name: item.name, size: item.size, type: item.type, modified: '刚刚', isFolder: false, originDeviceId: currentDeviceId, replicaDeviceIds: [currentDeviceId] }))
    setFiles(current => [...additions, ...current]); showNotice(`已添加 ${additions.length} 个文件`)
  }

  return (
    <div className="page-shell">
      <div className="mb-5 flex min-h-11 flex-wrap items-center justify-between gap-4">
        <div className="lg:hidden"><h1 className="page-title">全部文件</h1></div>
        <div className="hidden min-w-0 flex-1 items-center gap-1 overflow-x-auto whitespace-nowrap text-sm lg:flex">
          {path.length > 0 && <button onClick={() => setPath([])} className="icon-button h-11 w-11 shrink-0" aria-label="返回文件根目录" title="根目录"><FolderOpen size={18} /></button>}
          {path.map((folder, index) => <span key={`${folder}-${index}`} className="flex items-center gap-1"><ChevronRight size={15} className={`app-muted ${index === 0 ? 'hidden' : ''}`} /><button onClick={() => setPath(current => current.slice(0, index + 1))} className={`h-11 rounded-xl px-3 font-medium ${index === path.length - 1 ? 'bg-[var(--surface-soft)]' : 'app-muted hover:bg-[var(--surface-soft)] hover:text-primary-600'}`}>{folder}</button></span>)}
        </div>
        <div className="flex gap-2">
          <button className="btn btn-secondary px-3 sm:px-4" onClick={() => setFolderDialog(true)}><FolderPlus size={18} /><span className="hidden sm:inline">新建文件夹</span></button>
          <button className="btn btn-primary px-3 sm:px-4" onClick={() => fileInput.current?.click()}><Upload size={18} /><span className="hidden sm:inline">上传文件</span></button>
          <input ref={fileInput} className="hidden" type="file" multiple onChange={e => importFiles(e.target.files)} />
        </div>
      </div>

      {path.length > 0 && <div className="mb-4 flex min-h-10 items-center gap-1 overflow-x-auto whitespace-nowrap text-sm lg:hidden">
        <button onClick={() => setPath([])} className="app-muted rounded-lg px-2 py-1.5 font-medium hover:bg-[var(--surface)] hover:text-primary-600">我的空间</button>
        {path.map((folder, index) => <span key={`${folder}-${index}`} className="flex items-center gap-1"><ChevronRight size={15} className="app-muted" /><button onClick={() => setPath(current => current.slice(0, index + 1))} className={`rounded-lg px-2 py-1.5 font-medium ${index === path.length - 1 ? '' : 'app-muted hover:text-primary-600'}`}>{folder}</button></span>)}
      </div>}

      <div className="app-surface mb-4 flex flex-wrap items-center gap-2 rounded-2xl border p-2.5">
        <label className="relative min-w-[150px] flex-1 sm:flex-none">
          <span className="sr-only">文件类型</span>
          <select value={typeFilter} onChange={event => setTypeFilter(event.target.value as typeof typeFilter)} className="h-11 w-full appearance-none rounded-xl border border-[var(--border)] bg-[var(--surface-soft)] pl-4 pr-10 text-sm font-medium outline-none transition-colors hover:border-primary-300 focus:border-primary-500 focus:ring-2 focus:ring-primary-500/15">
            <option value="all">全部类型</option><option value="folder">文件夹</option><option value="image">图片</option><option value="document">文档</option><option value="video">视频</option><option value="audio">音频</option><option value="other">其他</option>
          </select>
          <ChevronDown size={16} className="app-muted pointer-events-none absolute right-3 top-1/2 -translate-y-1/2" />
        </label>
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
          <button className="btn btn-ghost min-h-9 px-2 text-primary-700 dark:text-primary-200"><Share2 size={16} />分享</button>
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
                {available && <button onClick={() => toggleSelected(file.id)} className={`absolute right-3 top-3 z-10 flex h-6 w-6 items-center justify-center rounded-lg border transition-opacity ${active ? 'border-primary-600 bg-primary-600 text-white' : 'border-[var(--border)] bg-[var(--surface)] opacity-100 sm:opacity-0 sm:group-hover:opacity-100'}`} aria-label={`选择 ${file.name}`}>{active && <Check size={14} />}</button>}
                {!file.isFolder && <button onClick={() => setDetailFileId(file.id)} className="icon-button absolute left-2 top-2 z-10 h-8 w-8 bg-[var(--surface)] opacity-100 shadow-sm sm:opacity-0 sm:group-hover:opacity-100" aria-label={`查看 ${file.name} 属性`}><MoreHorizontal size={17} /></button>}
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
                <span className="app-muted hidden text-sm sm:block">{file.isFolder ? '—' : formatFileSize(file.size)}</span><span className="app-muted hidden text-sm sm:block">{file.modified}</span>{file.isFolder ? <span /> : <button onClick={() => setDetailFileId(file.id)} className="icon-button" aria-label={`查看 ${file.name} 属性`}><MoreHorizontal size={18} /></button>}
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
          <button role="menuitem" disabled={!available} onClick={() => { downloadFile(file); setContextMenu(null) }} className="flex min-h-10 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium hover:bg-[var(--surface-soft)] disabled:cursor-not-allowed disabled:opacity-40"><Download size={17} />下载</button>
          <button role="menuitem" onClick={() => { setDetailFileId(file.id); setContextMenu(null) }} className="flex min-h-10 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium hover:bg-[var(--surface-soft)]"><Info size={17} />属性</button>
          <div className="my-1 border-t border-[var(--border)]" />
          <button role="menuitem" disabled={!available} onClick={() => { setSelected([file.id]); setConfirmDelete(true); setContextMenu(null) }} className="flex min-h-10 w-full items-center gap-3 rounded-lg px-3 text-sm font-medium text-red-600 hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-40 dark:hover:bg-red-950/30"><Trash2 size={17} />删除</button>
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

      {folderDialog && <div className="modal-backdrop fixed inset-0 z-50 flex items-end justify-center p-3 sm:items-center" onMouseDown={() => setFolderDialog(false)}><div role="dialog" aria-modal="true" aria-labelledby="folder-title" onMouseDown={e => e.stopPropagation()} className="modal-panel app-surface w-full max-w-md rounded-2xl border p-5 shadow-2xl"><div className="mb-5 flex items-center justify-between"><div><h2 id="folder-title" className="text-lg font-semibold">新建文件夹</h2><p className="app-muted mt-1 text-sm">文件夹将创建在当前位置</p></div><button className="icon-button" onClick={() => setFolderDialog(false)} aria-label="关闭"><X size={19} /></button></div><label className="mb-5 block text-sm font-medium">文件夹名称<input autoFocus value={folderName} onChange={e => setFolderName(e.target.value)} onKeyDown={e => e.key === 'Enter' && createFolder()} className="input mt-2" placeholder="例如：项目资料" /></label><div className="flex justify-end gap-2"><button className="btn btn-secondary" onClick={() => setFolderDialog(false)}>取消</button><button className="btn btn-primary" disabled={!folderName.trim()} onClick={createFolder}>创建</button></div></div></div>}
      <ConfirmDialog open={confirmDelete} title={`删除 ${selected.length} 个项目？`} confirmLabel="移至回收站" destructive onCancel={() => setConfirmDelete(false)} onConfirm={deleteSelectedFiles} />
      {notice && <div role="status" className="fixed bottom-24 left-1/2 z-50 flex -translate-x-1/2 items-center gap-2 rounded-xl bg-gray-900 px-4 py-3 text-sm font-medium text-white shadow-xl sm:bottom-6"><Check size={17} className="text-primary-300" />{notice}</div>}
    </div>
  )
}
