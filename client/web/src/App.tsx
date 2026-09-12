import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider, useAuth } from './contexts/AuthContext'
import { ReactNode } from 'react'
import { ThemeProvider } from './contexts/ThemeContext'
import DesktopLayout from './layouts/DesktopLayout'
import MobileLayout from './layouts/MobileLayout'
import LoginPage from './pages/Login'
import FilesPage from './pages/Files'
import TransfersPage from './pages/Transfers'
import DevicesPage from './pages/Devices'
import TrashPage from './pages/Trash'
import SharesPage from './pages/Shares'
import PublicShare from './pages/Shares/PublicShare'
import SettingsPage from './pages/Settings'
import useMediaQuery from './hooks/useMediaQuery'

function Authenticated({ children }: { children: ReactNode }) {
  const { loading, isAuthenticated, error } = useAuth()
  if (loading) return <main className="p-8" role="status">正在验证登录状态…</main>
  if (!isAuthenticated) return <Navigate to="/login" replace />
  return <>{error && <p role="alert" className="p-3 text-red-600">{error}</p>}{children}</>
}

function App() {
  const isDesktop = useMediaQuery('(min-width: 1025px)')

  return (
    <ThemeProvider>
      <AuthProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/s/:token" element={<PublicShare />} />
            <Route path="/login" element={<LoginPage />} />
            <Route
              path="/*"
              element={
                <Authenticated>{isDesktop ? (
                  <DesktopLayout>
                    <Routes>
                      <Route path="/" element={<Navigate to="/files" replace />} />
                      <Route path="/files" element={<FilesPage />} />
                      <Route path="/transfers" element={<TransfersPage />} />
                      <Route path="/devices" element={<DevicesPage />} />
                      <Route path="/trash" element={<TrashPage />} />
                      <Route path="/shares" element={<SharesPage />} />
                      <Route path="/settings" element={<SettingsPage />} />
                    </Routes>
                  </DesktopLayout>
                ) : (
                  <MobileLayout>
                    <Routes>
                      <Route path="/" element={<Navigate to="/files" replace />} />
                      <Route path="/files" element={<FilesPage />} />
                      <Route path="/transfers" element={<TransfersPage />} />
                      <Route path="/devices" element={<DevicesPage />} />
                      <Route path="/trash" element={<TrashPage />} />
                      <Route path="/shares" element={<SharesPage />} />
                      <Route path="/settings" element={<SettingsPage />} />
                    </Routes>
                  </MobileLayout>
                )}</Authenticated>
              }
            />
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </ThemeProvider>
  )
}

export default App
