import { useState } from 'react'
import { useParams } from 'react-router-dom'
import { Download, Share2 } from 'lucide-react'
import api from '../../services/api'
import { submitDownload } from '../../services/agentData'

export default function PublicShare() {
  const { token } = useParams()
  const [loading,setLoading] = useState(false)
  const [error,setError] = useState('')
  const download = async () => {
    setLoading(true); setError('')
    try {
      const result = await api.request<{ content_url: string; access_token: string }>(`/public/shares/${encodeURIComponent(token || '')}`)
      const url = new URL(result.content_url)
      url.pathname = '/v1/lan/browser-download'; url.search = ''; url.hash = ''
      submitDownload(url.href,result.access_token)
    } catch (cause) { setError((cause as Error).message) }
    finally { setLoading(false) }
  }
  return <main className="flex min-h-[100dvh] items-center justify-center p-5"><section className="card w-full max-w-md text-center"><Share2 className="mx-auto mb-5 text-primary-600" size={36} /><h1 className="text-2xl font-semibold">Share Disk 文件分享</h1><p className="app-muted my-4 text-sm">文件保存在分享者的设备中。下载时需要该设备在线且网络可达。</p>{error && <p role="alert" className="mb-4 text-sm text-red-600">{error}</p>}<button className="btn btn-primary w-full" disabled={loading} onClick={download}><Download size={18} />{loading ? '正在获取下载授权…' : '下载文件'}</button></section></main>
}
