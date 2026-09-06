import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from './contexts/AuthContext'
import { ThemeProvider } from './contexts/ThemeContext'
import DesktopLayout from './layouts/DesktopLayout'
import MobileLayout from './layouts/MobileLayout'
import LoginPage from './pages/Login'
import FilesPage from './pages/Files'
import TransfersPage from './pages/Transfers'
import DevicesPage from './pages/Devices'
import TrashPage from './pages/Trash'
import SharesPage from './pages/Shares'
import SettingsPage from './pages/Settings'
import useMediaQuery from './hooks/useMediaQuery'

function App() {
  const isDesktop = useMediaQuery('(min-width: 1025px)')

  return (
    <ThemeProvider>
      <AuthProvider>
        <BrowserRouter>
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route
              path="/*"
              element={
                isDesktop ? (
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
                )
              }
            />
          </Routes>
        </BrowserRouter>
      </AuthProvider>
    </ThemeProvider>
  )
}

export default App
