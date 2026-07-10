import React, { useState, useEffect } from 'react';
import api from '../api';
import { Plus, Trash2, Play } from 'lucide-react';

interface Workflow {
  id: string;
  name: string;
  version: number;
  definition: any;
  created_at: string;
}

interface ActivityDef {
  name: string;
  timeout: string;
  compensation?: string;
  retry_policy?: {
    max_attempts: number;
  };
}

const Workflows = () => {
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(0);
  const pageSize = 10;

  // Form state
  const [isCreating, setIsCreating] = useState(false);
  const [newName, setNewName] = useState('');
  const [activities, setActivities] = useState<ActivityDef[]>([]);

  const fetchWorkflows = async () => {
    try {
      const res = await api.get(`/workflows?limit=${pageSize}&offset=${page * pageSize}`);
      setWorkflows(res.data);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchWorkflows();
  }, [page]);

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      const parsedDef = { activities };
      await api.post('/workflows', {
        name: newName,
        definition: parsedDef
      });
      setIsCreating(false);
      setNewName('');
      setActivities([]);
      fetchWorkflows();
    } catch (e: any) {
      alert('Failed to create: ' + (e.response?.data || e.message));
    }
  };

  const startWorkflow = async (id: string, name: string) => {
    try {
      await api.post('/workflows/start', { workflow_id: id });
      alert(`Started workflow ${name}`);
    } catch (e: any) {
      alert('Failed to start: ' + e.message);
    }
  };

  const deleteWorkflow = async (id: string, name: string) => {
    if (!window.confirm(`Are you sure you want to delete workflow ${name}? This will delete all associated executions and schedules.`)) return;
    try {
      await api.delete(`/workflows/${id}`);
      fetchWorkflows();
    } catch (e: any) {
      alert('Failed to delete: ' + (e.response?.data || e.message));
    }
  };

  const addActivity = () => {
    setActivities([...activities, { name: '', timeout: '5m' }]);
  };

  const updateActivity = (index: number, field: string, value: any) => {
    const newActs = [...activities];
    if (field === 'max_attempts') {
      if (value) {
        newActs[index].retry_policy = { max_attempts: parseInt(value) };
      } else {
        delete newActs[index].retry_policy;
      }
    } else {
      (newActs[index] as any)[field] = value;
    }
    setActivities(newActs);
  };

  const removeActivity = (index: number) => {
    setActivities(activities.filter((_, i) => i !== index));
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-6">
        <h1 className="page-title mb-0">Workflows</h1>
        <button className="btn btn-primary" onClick={() => setIsCreating(!isCreating)}>
          <Plus size={16} /> New Workflow
        </button>
      </div>

      {isCreating && (
        <div className="card mb-8">
          <h2 className="text-xl font-bold mb-4">Create New Workflow</h2>
          <form onSubmit={handleCreate}>
            <div className="form-group">
              <label className="form-label">Workflow Name</label>
              <input className="input" value={newName} onChange={e => setNewName(e.target.value)} required placeholder="e.g. order-processing" />
            </div>
            
            <div className="mb-4">
              <div className="flex justify-between items-center mb-2">
                <label className="form-label mb-0">Activities</label>
                <button type="button" className="btn btn-neutral text-xs py-1 px-2" onClick={addActivity}>
                  <Plus size={12} className="mr-1 inline" /> Add Activity
                </button>
              </div>
              
              {activities.length === 0 && (
                <div className="p-4 border border-dashed border-gray-700 rounded text-center text-gray-500 text-sm">
                  No activities added yet.
                </div>
              )}

              {activities.map((act, i) => (
                <div key={i} className="p-4 border border-gray-700 rounded mb-2 bg-gray-900">
                  <div className="flex justify-between items-start mb-4">
                    <h4 className="font-bold">Activity {i + 1}</h4>
                    <button type="button" onClick={() => removeActivity(i)} className="text-danger-color hover:text-red-400">
                      <Trash2 size={16} />
                    </button>
                  </div>
                  
                  <div className="grid grid-cols-2 gap-4">
                    <div className="form-group mb-0">
                      <label className="form-label text-xs">Name</label>
                      <input className="input py-1 px-2 text-sm" value={act.name} onChange={e => updateActivity(i, 'name', e.target.value)} required placeholder="e.g. charge-card" />
                    </div>
                    <div className="form-group mb-0">
                      <label className="form-label text-xs">Timeout</label>
                      <input className="input py-1 px-2 text-sm" value={act.timeout} onChange={e => updateActivity(i, 'timeout', e.target.value)} required placeholder="e.g. 5m" />
                    </div>
                    <div className="form-group mb-0">
                      <label className="form-label text-xs">Compensation (Optional)</label>
                      <input className="input py-1 px-2 text-sm" value={act.compensation || ''} onChange={e => updateActivity(i, 'compensation', e.target.value)} placeholder="e.g. refund-card" />
                    </div>
                    <div className="form-group mb-0">
                      <label className="form-label text-xs">Max Retries (Optional)</label>
                      <input type="number" min="0" className="input py-1 px-2 text-sm" value={act.retry_policy?.max_attempts || ''} onChange={e => updateActivity(i, 'max_attempts', e.target.value)} placeholder="e.g. 3" />
                    </div>
                  </div>
                </div>
              ))}
            </div>

            <div className="flex gap-4">
              <button type="submit" className="btn btn-primary">Create Workflow</button>
              <button type="button" className="btn" onClick={() => setIsCreating(false)}>Cancel</button>
            </div>
          </form>
        </div>
      )}

      {loading ? (
        <div>Loading workflows...</div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Version</th>
                <th>Created At</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {workflows.map((wf) => (
                <tr key={`${wf.name}-${wf.version}`}>
                  <td className="font-medium">{wf.name}</td>
                  <td>v{wf.version}</td>
                  <td>{new Date(wf.created_at).toLocaleString()}</td>
                  <td>
                    <div className="flex gap-2">
                      <button className="btn btn-primary text-xs py-1" onClick={() => startWorkflow(wf.id, wf.name)}>
                        <Play size={12} className="mr-1 inline" /> Start
                      </button>
                      <button className="btn btn-danger text-xs py-1" onClick={() => deleteWorkflow(wf.id, wf.name)}>
                        <Trash2 size={12} className="mr-1 inline" /> Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {workflows.length === 0 && (
                <tr>
                  <td colSpan={4} className="text-center py-8 text-gray-500">No workflows found</td>
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
              disabled={workflows.length < pageSize} 
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

export default Workflows;
