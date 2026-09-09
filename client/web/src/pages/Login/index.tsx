import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowRight, CheckCircle2, Eye, EyeOff, HardDrive, Loader2, LockKeyhole } from 'lucide-react'
import { useAuth } from '../../contexts/AuthContext'

export default function LoginPage() {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const { login } = useAuth()
  const navigate = useNavigate()

  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setError('')
    if (!username.trim() || password.length < 4) { setError('请输入用户名和至少 4 位密码'); return }
    setLoading(true)
    try { await login(username.trim(), password); navigate('/files') }
    catch { setError('暂时无法登录，请稍后重试') }
    finally { setLoading(false) }
  }

  return (
    <main className="grid min-h-[100dvh] bg-[var(--surface)] lg:grid-cols-[1.05fr_.95fr]">
      <section className="relative hidden overflow-hidden bg-[#0d211c] p-12 text-white lg:flex lg:flex-col">
        <div className="absolute -right-28 -top-28 h-96 w-96 rounded-full border border-white/10" />
        <div className="absolute -right-4 top-20 h-64 w-64 rounded-full border border-white/10" />
        <div className="relative flex items-center gap-3"><span className="flex h-11 w-11 items-center justify-center rounded-[15px] bg-primary-500"><HardDrive size={22} /></span><span className="text-lg font-semibold">Share Disk</span></div>
        <div className="relative my-auto max-w-xl">
          <p className="mb-4 text-sm font-semibold uppercase tracking-[.2em] text-primary-300">由你的设备共同守护</p>
          <h1 className="text-4xl font-semibold leading-[1.15] tracking-tight xl:text-5xl">文件留在身边，<br />随时安全访问。</h1>
          <p className="mt-6 max-w-md text-base leading-7 text-white/65">不依赖公共云盘，把闲置设备连接成你的私人存储空间。</p>
        </div>
        <div className="relative flex gap-8 text-sm text-white/60"><span className="flex items-center gap-2"><CheckCircle2 size={16} className="text-primary-300" />端到端传输</span><span className="flex items-center gap-2"><CheckCircle2 size={16} className="text-primary-300" />本地数据所有权</span></div>
      </section>

      <section className="flex items-center justify-center px-5 py-10 sm:px-10">
        <div className="w-full max-w-sm">
          <div className="mb-10 flex items-center gap-3 lg:hidden"><span className="flex h-10 w-10 items-center justify-center rounded-[14px] bg-primary-600 text-white"><HardDrive size={20} /></span><span className="font-semibold">Share Disk</span></div>
          <span className="mb-5 flex h-11 w-11 items-center justify-center rounded-xl bg-primary-50 text-primary-700 dark:bg-primary-900/50 dark:text-primary-300"><LockKeyhole size={20} /></span>
          <h1 className="text-3xl font-semibold tracking-tight">欢迎回来</h1>
          <p className="app-muted mt-2">登录以访问你的私人空间</p>

          <form onSubmit={submit} className="mt-8 space-y-5">
            <label className="block text-sm font-medium">用户名<input autoComplete="username" value={username} onChange={e => setUsername(e.target.value)} className="input mt-2" placeholder="请输入用户名" /></label>
            <label className="block text-sm font-medium">密码<span className="relative mt-2 block"><input autoComplete="current-password" type={showPassword ? 'text' : 'password'} value={password} onChange={e => setPassword(e.target.value)} className="input pr-12" placeholder="请输入密码" /><button type="button" onClick={() => setShowPassword(v => !v)} className="icon-button absolute right-1 top-1/2 -translate-y-1/2" aria-label={showPassword ? '隐藏密码' : '显示密码'}>{showPassword ? <EyeOff size={18} /> : <Eye size={18} />}</button></span></label>
            {error && <p role="alert" className="rounded-xl bg-red-50 px-3 py-2.5 text-sm text-red-700 dark:bg-red-950/40 dark:text-red-300">{error}</p>}
            <div className="flex items-center justify-between text-sm"><label className="app-muted flex items-center gap-2"><input type="checkbox" className="h-4 w-4 accent-primary-600" />保持登录</label><button type="button" className="font-medium text-primary-700 hover:underline dark:text-primary-300">忘记密码？</button></div>
            <button disabled={loading} className="btn btn-primary w-full">{loading ? <Loader2 size={18} className="animate-spin" /> : <>登录 <ArrowRight size={18} /></>}</button>
          </form>
          <p className="app-muted mt-8 text-center text-xs">Share Disk 仅在你的私有网络中工作</p>
        </div>
      </section>
    </main>
  )
}
