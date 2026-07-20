import { useState, useEffect } from 'react';
import api from '../api';
import { Plus, Trash2, Pause, PlayCircle } from 'lucide-react';

const Schedules = () => {
  const [schedules, setSchedules] = useState<any[]>([]);
  const [workflows, setWorkflows] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  // Form state
  const [isCreating, setIsCreating] = useState(false);
  const [wfId, setWfId] = useState('');
  const [cron, setCron] = useState('');
  const [inputData, setInputData] = useState('{}');

  const fetchData = async () => {
    try {
      const [schRes, wfRes] = await Promise.all([
        api.get('/schedules'),
        api.get('/workflows')
      ]);
      setSchedules(schRes.data || []);
      setWorkflows(wfRes.data || []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!wfId) {
      alert('Please select a workflow');
      return;
    }
    try {
      await api.post('/schedules', {
        workflow_id: wfId,
        cron_expression: cron,
        schedule_type: 'recurring',
        input: JSON.parse(inputData)
      });
      setIsCreating(false);
      setWfId('');
      setCron('');
      setInputData('{}');
      fetchData();
    } catch (e: any) {
      alert('Failed to create: ' + (e.response?.data || e.message));
    }
  };

  const deleteSchedule = async (id: string) => {
    if (!window.confirm('Delete this schedule?')) return;
    try {
      await api.delete(`/schedules/${id}`);
      fetchData();
    } catch (e: any) {
      alert('Failed to delete: ' + e.message);
    }
  };

  const pauseSchedule = async (id: string) => {
    try {
      await api.post(`/schedules/${id}/pause`);
      fetchData();
    } catch (e: any) {
      alert('Failed to pause: ' + e.message);
    }
  };

  const resumeSchedule = async (id: string) => {
    try {
      await api.post(`/schedules/${id}/resume`);
      fetchData();
    } catch (e: any) {
      alert('Failed to resume: ' + e.message);
    }
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-8">
        <h1 className="page-title mb-0">Schedules</h1>
        <button className="btn btn-primary px-4 py-2" onClick={() => setIsCreating(!isCreating)}>
          <Plus size={16} className="mr-2 inline" /> New Schedule
        </button>
      </div>

      {isCreating && (
        <div className="card mb-8">
          <h2 className="text-xl font-bold mb-4">Create New Schedule</h2>
          <form onSubmit={handleCreate}>
            <div className="form-group">
              <label className="form-label">Workflow</label>
              <select className="input" value={wfId} onChange={e => setWfId(e.target.value)} required>
                <option value="" disabled>Select a workflow</option>
                {workflows.map(wf => (
                  <option key={wf.id} value={wf.id}>{wf.name}</option>
                ))}
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">Cron Expression (e.g. 0 * * * *)</label>
              <input className="input" value={cron} onChange={e => setCron(e.target.value)} required />
            </div>
            <div className="form-group">
              <label className="form-label">Input Data (JSON)</label>
              <textarea 
                className="input" 
                rows={4} 
                value={inputData} 
                onChange={e => setInputData(e.target.value)} 
                style={{ fontFamily: 'monospace' }}
              />
            </div>
            <div className="flex gap-4">
              <button type="submit" className="btn btn-primary">Create</button>
              <button type="button" className="btn btn-secondary" onClick={() => setIsCreating(false)}>Cancel</button>
            </div>
          </form>
        </div>
      )}

      {loading ? (
        <div>Loading schedules...</div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>ID</th>
                <th>Workflow Name</th>
                <th>Cron Expression</th>
                <th>Next Run Time</th>
                <th>Status</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {schedules.map((sch) => (
                <tr key={sch.id}>
                  <td className="font-mono text-xs">{sch.id}</td>
                  <td className="font-medium">{workflows.find(w => w.id === sch.workflow_id)?.name || sch.workflow_id}</td>
                  <td className="font-mono text-sm">{sch.cron_expression}</td>
                  <td>{sch.next_run_at ? new Date(sch.next_run_at.Time || sch.next_run_at).toLocaleString() : 'N/A'}</td>
                  <td>
                    <span className={`status-badge ${sch.status === 'ACTIVE' ? 'status-success' : sch.status === 'PAUSED' ? 'status-paused' : 'status-pending'}`}>
                      {sch.status}
                    </span>
                  </td>
                  <td>
                    <div className="flex gap-3">
                      {sch.status === 'ACTIVE' ? (
                        <button className="btn btn-secondary px-3 py-1.5 text-sm text-warning-color border-warning-color" onClick={() => pauseSchedule(sch.id)}>
                          <Pause size={14} className="mr-1 inline" /> Pause
                        </button>
                      ) : (
                        <button className="btn btn-secondary px-3 py-1.5 text-sm text-success-color border-success-color" onClick={() => resumeSchedule(sch.id)}>
                          <PlayCircle size={14} className="mr-1 inline" /> Resume
                        </button>
                      )}
                      <button className="btn btn-danger px-3 py-1.5 text-sm" onClick={() => deleteSchedule(sch.id)}>
                        <Trash2 size={14} className="mr-1 inline" /> Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {schedules.length === 0 && (
                <tr>
                  <td colSpan={6} className="text-center py-8 text-gray-500">No schedules found</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
};

export default Schedules;
