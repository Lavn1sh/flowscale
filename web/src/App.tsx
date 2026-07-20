import React from 'react';
import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom';
import { useAuth } from './context/AuthContext';
import { LayoutDashboard, Play, Activity, Clock, LogOut } from 'lucide-react';
import { NavLink } from 'react-router-dom';
import { DemoControls } from './components/DemoControls';

// Pages (to be implemented)
import Login from './pages/Login';
import Workflows from './pages/Workflows';
import Executions from './pages/Executions';
import ExecutionDetail from './pages/ExecutionDetail';
import DLQ from './pages/DLQ';
import Schedules from './pages/Schedules';

const Sidebar = () => {
  const { logout, user } = useAuth();
  
  return (
    <div className="sidebar">
      <div className="sidebar-header">
        <Activity className="text-indigo-500" />
        <span>Flowscale</span>
      </div>
      <div className="sidebar-nav">
        <NavLink to="/workflows" className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}>
          <LayoutDashboard size={20} />
          <span>Workflows</span>
        </NavLink>
        <NavLink to="/executions" className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}>
          <Play size={20} />
          <span>Executions</span>
        </NavLink>
        <NavLink to="/dlq" className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}>
          <Activity size={20} />
          <span>DLQ / Compensations</span>
        </NavLink>
        <NavLink to="/schedules" className={({ isActive }) => `nav-item ${isActive ? 'active' : ''}`}>
          <Clock size={20} />
          <span>Schedules</span>
        </NavLink>
      </div>
      <div className="p-6 pb-8 border-t border-gray-800 mt-auto">
        <div className="text-sm text-gray-400 mb-4">Logged in as {user?.username}</div>
        <button className="btn btn-danger w-full py-2.5" onClick={logout}>
          <LogOut size={16} /> Logout
        </button>
      </div>
    </div>
  );
};

const MainLayout = () => {
  return (
    <div className="app-container">
      <Sidebar />
      <div className="main-content">
        <div className="topbar flex justify-end items-center w-full px-6">
          <DemoControls />
        </div>
        <div className="content-area">
          <Outlet />
        </div>
      </div>
    </div>
  );
};

const ProtectedRoute = ({ children }: { children: React.ReactNode }) => {
  const { token } = useAuth();
  if (!token) return <Navigate to="/login" replace />;
  return <>{children}</>;
};

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        
        <Route path="/" element={
          <ProtectedRoute>
            <MainLayout />
          </ProtectedRoute>
        }>
          <Route index element={<Navigate to="/workflows" replace />} />
          <Route path="workflows" element={<Workflows />} />
          <Route path="executions" element={<Executions />} />
          <Route path="executions/:id" element={<ExecutionDetail />} />
          <Route path="dlq" element={<DLQ />} />
          <Route path="schedules" element={<Schedules />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}

export default App;
