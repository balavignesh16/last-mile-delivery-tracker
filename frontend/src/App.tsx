import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { ProtectedRoute } from './components/ProtectedRoute'
import { AuthProvider } from './contexts/AuthContext'
import { Account } from './pages/Account'
import { AgentsPage } from './pages/admin/AgentsPage'
import { RatesPage } from './pages/admin/RatesPage'
import { ZonesPage } from './pages/admin/ZonesPage'
import { OperationsPage } from './pages/agent/OperationsPage'
import { Home } from './pages/Home'
import { LoginPage } from './pages/LoginPage'
import { QuotePage } from './pages/QuotePage'
import { RegisterPage } from './pages/RegisterPage'

function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/" element={<Home />} />
          <Route path="/login" element={<LoginPage />} />
          <Route path="/register" element={<RegisterPage />} />
          <Route
            path="/app"
            element={
              <ProtectedRoute>
                <Account />
              </ProtectedRoute>
            }
          />
          <Route
            path="/admin/agents"
            element={
              <ProtectedRoute roles={['ADMIN']}>
                <AgentsPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/admin/zones"
            element={
              <ProtectedRoute roles={['ADMIN']}>
                <ZonesPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/admin/rates"
            element={
              <ProtectedRoute roles={['ADMIN']}>
                <RatesPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/agent"
            element={
              <ProtectedRoute roles={['DELIVERY_AGENT']}>
                <OperationsPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/quote"
            element={
              <ProtectedRoute roles={['ADMIN', 'CUSTOMER']}>
                <QuotePage />
              </ProtectedRoute>
            }
          />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}

export default App
