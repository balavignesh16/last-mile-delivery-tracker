import { BrowserRouter, Route, Routes } from 'react-router-dom'
import { ProtectedRoute } from './components/ProtectedRoute'
import { AuthProvider } from './contexts/AuthContext'
import { Account } from './pages/Account'
import { AgentsPage } from './pages/admin/AgentsPage'
import { DashboardPage as AdminDashboardPage } from './pages/admin/DashboardPage'
import { RatesPage } from './pages/admin/RatesPage'
import { ZonesPage } from './pages/admin/ZonesPage'
import { DashboardPage as AgentDashboardPage } from './pages/agent/DashboardPage'
import { OperationsPage } from './pages/agent/OperationsPage'
import { CreateOrderPage } from './pages/CreateOrderPage'
import { DashboardPage as CustomerDashboardPage } from './pages/customer/DashboardPage'
import { Home } from './pages/Home'
import { LoginPage } from './pages/LoginPage'
import { OrderDetailPage } from './pages/OrderDetailPage'
import { OrdersPage } from './pages/OrdersPage'
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
            path="/customer/dashboard"
            element={
              <ProtectedRoute roles={['CUSTOMER']}>
                <CustomerDashboardPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/agent/dashboard"
            element={
              <ProtectedRoute roles={['DELIVERY_AGENT']}>
                <AgentDashboardPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/admin/dashboard"
            element={
              <ProtectedRoute roles={['ADMIN']}>
                <AdminDashboardPage />
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
          <Route
            path="/orders"
            element={
              <ProtectedRoute roles={['ADMIN', 'CUSTOMER', 'DELIVERY_AGENT']}>
                <OrdersPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/orders/new"
            element={
              <ProtectedRoute roles={['ADMIN', 'CUSTOMER']}>
                <CreateOrderPage />
              </ProtectedRoute>
            }
          />
          <Route
            path="/orders/:id"
            element={
              <ProtectedRoute roles={['ADMIN', 'CUSTOMER', 'DELIVERY_AGENT']}>
                <OrderDetailPage />
              </ProtectedRoute>
            }
          />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  )
}

export default App
