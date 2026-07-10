import { useState, useEffect } from 'react';
import api from '../api';
import { Plus, Trash2 } from 'lucide-react';

const Schedules = () => {
  const [schedules, setSchedules] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  // Form state
  const [isCreating, setIsCreating] = useState(false);
  const [wfName, setWfName] = useState('');
  const [cron, setCron] = useState('');
  const [inputData, setInputData] = useState('{}');

  const fetchSchedules = async () => {
    try {
      const res = await api.get('/schedules');
      setSchedules(res.data || []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSchedules();
  }, []);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      await api.post('/schedules', {
        workflow_name: wfName,
        cron_expression: cron,
        input: JSON.parse(inputData)
      });
      setIsCreating(false);
      setWfName('');
      setCron('');
      setInputData('{}');
      fetchSchedules();
    } catch (e: any) {
      alert('Failed to create: ' + (e.response?.data || e.message));
    }
  };

  const deleteSchedule = async (id: string) => {
    if (!window.confirm('Delete this schedule?')) return;
    try {
      await api.delete(`/schedules/${id}`);
      fetchSchedules();
    } catch (e: any) {
      alert('Failed to delete: ' + e.message);
    }
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <h1 className="page-title mb-0">Schedules</h1>
        <button className="btn btn-primary" onClick={() => setIsCreating(!isCreating)}>
          <Plus size={16} /> New Schedule
        </button>
      </div>

      {isCreating && (
        <div className="card mb-8">
          <h2 className="text-xl font-bold mb-4">Create New Schedule</h2>
          <form onSubmit={handleCreate}>
            <div className="form-group">
              <label className="form-label">Workflow Name</label>
              <input className="input" value={wfName} onChange={e => setWfName(e.target.value)} required />
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
              <button type="button" className="btn" onClick={() => setIsCreating(false)}>Cancel</button>
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
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {schedules.map((sch) => (
                <tr key={sch.id}>
                  <td className="font-mono text-xs">{sch.id}</td>
                  <td className="font-medium">{sch.workflow_name}</td>
                  <td className="font-mono text-sm">{sch.cron_expression}</td>
                  <td>{sch.next_run_at ? new Date(sch.next_run_at.Time || sch.next_run_at).toLocaleString() : 'N/A'}</td>
                  <td>
                    <button className="btn btn-danger text-xs py-1 px-2" onClick={() => deleteSchedule(sch.id)}>
                      <Trash2 size={12} className="mr-1 inline" /> Delete
                    </button>
                  </td>
                </tr>
              ))}
              {schedules.length === 0 && (
                <tr>
                  <td colSpan={5} className="text-center py-8 text-gray-500">No schedules found</td>
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
