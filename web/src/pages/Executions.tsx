import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import api from '../api';
import { Trash2 } from 'lucide-react';

interface Execution {
  id: string;
  workflow_name: string;
  status: string;
  created_at: string;
  updated_at: string;
}

const Executions = () => {
  const [executions, setExecutions] = useState<Execution[]>([]);
  const [loading, setLoading] = useState(true);
  const [statusFilter, setStatusFilter] = useState('');
  const [workflowFilter, setWorkflowFilter] = useState('');
  const [page, setPage] = useState(0);
  const pageSize = 10;
  
  const fetchExecutions = async () => {
    try {
      const params = new URLSearchParams();
      if (statusFilter) params.append('status', statusFilter);
      if (workflowFilter) params.append('workflow_id', workflowFilter);
      params.append('limit', pageSize.toString());
      params.append('offset', (page * pageSize).toString());
      
      const res = await api.get(`/executions?${params.toString()}`);
      setExecutions(res.data);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchExecutions();
  }, [statusFilter, workflowFilter, page]);

  const getStatusBadgeClass = (status: string) => {
    switch(status) {
      case 'COMPLETED': return 'badge-success';
      case 'FAILED': return 'badge-danger';
      case 'RUNNING': return 'badge-info';
      case 'COMPENSATING': return 'badge-warning';
      case 'CANCELLED': return 'badge-neutral';
      default: return 'badge-neutral';
    }
  };

  const cancelExecution = async (id: string) => {
    if (!window.confirm('Cancel this execution?')) return;
    try {
      await api.post(`/executions/${id}/cancel`);
      fetchExecutions();
    } catch (e: any) {
      alert('Failed to cancel: ' + e.message);
    }
  };

  const deleteExecution = async (id: string) => {
    if (!window.confirm('Delete this execution permanently?')) return;
    try {
      await api.delete(`/executions/${id}`);
      fetchExecutions();
    } catch (e: any) {
      alert('Failed to delete: ' + (e.response?.data || e.message));
    }
  };

  return (
    <div>
      <h1 className="page-title">Executions</h1>
      
      <div className="card mb-6 flex gap-4 p-4 items-end">
        <div className="flex-1">
          <label className="form-label mb-1">Status Filter</label>
          <select 
            className="input" 
            value={statusFilter} 
            onChange={e => setStatusFilter(e.target.value)}
          >
            <option value="">All Statuses</option>
            <option value="RUNNING">Running</option>
            <option value="COMPLETED">Completed</option>
            <option value="FAILED">Failed</option>
            <option value="COMPENSATING">Compensating</option>
            <option value="CANCELLED">Cancelled</option>
          </select>
        </div>
        <div className="flex-1">
          <label className="form-label mb-1">Workflow Filter</label>
          <input 
            type="text" 
            className="input" 
            placeholder="Workflow Name" 
            value={workflowFilter} 
            onChange={e => setWorkflowFilter(e.target.value)}
          />
        </div>
        <button className="btn btn-primary" onClick={fetchExecutions}>Refresh</button>
      </div>

      {loading ? (
        <div>Loading executions...</div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>Workflow</th>
                <th>Status</th>
                <th>Created At</th>
                <th>Updated At</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {executions.map((ex) => (
                <tr key={ex.id}>
                  <td className="font-mono text-xs">
                    <Link to={`/executions/${ex.id}`} className="hover:underline">{ex.id}</Link>
                  </td>
                  <td className="font-medium">{ex.workflow_name}</td>
                  <td>
                    <span className={`badge ${getStatusBadgeClass(ex.status)}`}>
                      {ex.status}
                    </span>
                  </td>
                  <td>{new Date(ex.created_at).toLocaleString()}</td>
                  <td>{new Date(ex.updated_at).toLocaleString()}</td>
                  <td>
                    <div className="flex gap-2">
                      {ex.status === 'RUNNING' && (
                        <button className="btn btn-warning text-xs py-1 px-2" onClick={() => cancelExecution(ex.id)}>
                          Cancel
                        </button>
                      )}
                      <button className="btn btn-danger text-xs py-1 px-2" onClick={() => deleteExecution(ex.id)}>
                        <Trash2 size={12} className="mr-1 inline" /> Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {executions.length === 0 && (
                <tr>
                  <td colSpan={6} className="text-center py-8 text-gray-500">No executions found</td>
                </tr>
              )}
            </tbody>
          </table>
          
          <div className="flex justify-between items-center mt-4">
            <button 
              className="btn" 
              disabled={page === 0} 
              onClick={() => setPage(p => Math.max(0, p - 1))}
            >
              Previous
            </button>
            <span className="text-sm text-gray-400">Page {page + 1}</span>
            <button 
              className="btn" 
              disabled={executions.length < pageSize} 
              onClick={() => setPage(p => p + 1)}
            >
              Next
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

export default Executions;
