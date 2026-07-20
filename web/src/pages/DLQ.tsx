import { useState, useEffect } from 'react';
import api from '../api';
import { RotateCcw } from 'lucide-react';

const DLQ = () => {
  const [dlqItems, setDlqItems] = useState<any[]>([]);
  const [compensations, setCompensations] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchData = async () => {
    try {
      const [dlqRes, compRes] = await Promise.all([
        api.get('/activities/dlq'),
        api.get('/executions?status=FAILED') // Just fetch failed executions to simulate pending compensations
      ]);
      setDlqItems(dlqRes.data || []);
      setCompensations(compRes.data || []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const retryDLQ = async (id: string) => {
    try {
      await api.post(`/activities/dlq/${id}/retry`);
      fetchData();
    } catch (e: any) {
      alert('Failed to retry: ' + e.message);
    }
  };

  const retryCompensation = async (id: string) => {
    try {
      await api.post(`/executions/${id}/compensate/retry`);
      fetchData();
    } catch (e: any) {
      alert('Failed to trigger compensation: ' + e.message);
    }
  };

  if (loading) return <div>Loading...</div>;

  return (
    <div>
      <h1 className="page-title">DLQ & Compensations</h1>
      
      <div className="card mb-10 shadow-sm border-gray-700/60">
        <h2 className="text-2xl font-bold mb-6 text-danger-color">Dead Letter Queue (DLQ)</h2>
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Execution ID</th>
                <th>Activity Name</th>
                <th>Failed At</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {dlqItems.map(item => (
                <tr key={item.id}>
                  <td className="font-mono text-xs">{item.execution_id}</td>
                  <td className="font-medium">{item.activity_name}</td>
                  <td>{new Date(item.dead_lettered_at?.Time || item.dead_lettered_at).toLocaleString()}</td>
                  <td>
                    <button className="btn btn-secondary px-3 py-1.5 text-sm" onClick={() => retryDLQ(item.id)}>
                      <RotateCcw size={14} className="mr-1 inline" /> Retry
                    </button>
                  </td>
                </tr>
              ))}
              {dlqItems.length === 0 && (
                <tr>
                  <td colSpan={4} className="text-center py-8 text-gray-500">No items in DLQ</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>

      <div className="card shadow-sm border-gray-700/60">
        <h2 className="text-2xl font-bold mb-2 text-warning-color">Pending Compensations</h2>
        <p className="text-sm text-gray-400 mb-6">Executions that failed and may need manual compensation trigger.</p>
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Execution ID</th>
                <th>Workflow Name</th>
                <th>Failed At</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {compensations.map(comp => (
                <tr key={comp.id}>
                  <td className="font-mono text-xs">{comp.id}</td>
                  <td className="font-medium">{comp.workflow_name}</td>
                  <td>{new Date(comp.updated_at).toLocaleString()}</td>
                  <td>
                    <button className="btn btn-warning text-xs py-1 bg-amber-500 text-white" onClick={() => retryCompensation(comp.id)}>
                      <RotateCcw size={12} className="mr-1 inline" /> Retry Compensation
                    </button>
                  </td>
                </tr>
              ))}
              {compensations.length === 0 && (
                <tr>
                  <td colSpan={4} className="text-center py-8 text-gray-500">No pending compensations found</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
};

export default DLQ;
