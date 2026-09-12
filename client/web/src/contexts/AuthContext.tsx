import { createContext, useContext, useEffect, useState, ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import api, { ApiError, BrowserSession } from '../services/api'
interface AuthContextType {
  user: { id: string; username: string } | null
  isAuthenticated: boolean
  loading: boolean
  error: string
  login: (account: string, password: string, remember?: boolean) => Promise<void>
  logout: () => Promise<void>
}
const AuthContext = createContext<AuthContextType | null>(null)
export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<BrowserSession | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const cache = useQueryClient()
  useEffect(() => {
    let active = true
    localStorage.removeItem('user')
    api.session().then(value => { if (active) setSession(value) }).catch(cause => {
      if (active && !(cause instanceof ApiError && cause.status === 401)) setError(cause.message)
    }).finally(() => { if (active) setLoading(false) })
    const expired = () => { setSession(null); cache.clear() }
    window.addEventListener('share-disk-session-expired', expired)
    return () => { active = false; window.removeEventListener('share-disk-session-expired', expired) }
  }, [cache])
  const login = async (account: string, password: string, remember = false) => {
    const value = await api.login(account, password, remember)
    cache.clear(); setSession(value); setError('')
  }
  const logout = async () => {
    try { await api.logout(); cache.clear(); setSession(null); setError('') }
    catch (cause) { setError((cause as Error).message) }
  }
  const user = session ? { id: session.account.id, username: session.account.account } : null
  return <AuthContext.Provider value={{ user, isAuthenticated: !!session, loading, error, login, logout }}>{children}</AuthContext.Provider>
}
export function useAuth() {
  const context = useContext(AuthContext)
  if (!context) throw new Error('AuthProvider is required')
  return context
}
