import React, { useState, useEffect } from 'react';
import api from '../api';
import { Plus, Trash2, Play, Edit2, ChevronDown, ChevronUp } from 'lucide-react';

interface Workflow {
  id: string;
  name: string;
  created_at: string;
}

interface ActivityDef {
  name: string;
  timeout: string;
  compensation?: string;
  retry_policy?: {
    max_attempts: number;
    initial_interval?: string;
    backoff_coefficient?: number;
  };
}

const Workflows = () => {
  const [workflows, setWorkflows] = useState<Workflow[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(0);
  const pageSize = 10;

  // Form state
  const [isFormOpen, setIsFormOpen] = useState(false);
  const [editingWorkflowId, setEditingWorkflowId] = useState<string | null>(null);
  const [newName, setNewName] = useState('');
  const [activities, setActivities] = useState<ActivityDef[]>([]);
  
  // UI state for collapsible activities
  const [expandedActivityIndex, setExpandedActivityIndex] = useState<number | null>(null);

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

  const handleOpenCreate = () => {
    setEditingWorkflowId(null);
    setNewName('');
    setActivities([]);
    setIsFormOpen(true);
    setExpandedActivityIndex(null);
  };

  const handleOpenEdit = async (id: string) => {
    try {
      const res = await api.get(`/workflows/${id}`);
      const wf = res.data;
      setEditingWorkflowId(wf.id);
      setNewName(wf.name);
      setActivities(wf.activities || []);
      setIsFormOpen(true);
      setExpandedActivityIndex(null);
    } catch (e: any) {
      alert('Failed to fetch workflow details: ' + (e.response?.data || e.message));
    }
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    try {
      if (editingWorkflowId) {
        await api.put(`/workflows/${editingWorkflowId}`, {
          name: newName,
          activities: activities
        });
      } else {
        await api.post('/workflows', {
          name: newName,
          activities: activities
        });
      }
      setIsFormOpen(false);
      fetchWorkflows();
    } catch (e: any) {
      alert(`Failed to ${editingWorkflowId ? 'update' : 'create'}: ` + (e.response?.data || e.message));
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

  const batchWorkflow = async (id: string, name: string) => {
    try {
      const promises = [];
      for (let i = 0; i < 10; i++) {
        promises.push(api.post('/workflows/start', { workflow_id: id }));
      }
      await Promise.all(promises);
      alert(`Started 10x batch execution for ${name}`);
    } catch (e: any) {
      alert('Failed to batch start: ' + e.message);
    }
  };

  const seedWorkflows = async () => {
    try {
      await api.post('/workflows/seed');
      alert('Seed complete!');
      fetchWorkflows();
    } catch (e: any) {
      alert('Failed to seed: ' + e.message);
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
    setExpandedActivityIndex(activities.length); // auto expand the new activity
  };

  const updateActivity = (index: number, field: string, value: any) => {
    const newActs = [...activities];
    if (['max_attempts', 'initial_interval', 'backoff_coefficient'].includes(field)) {
      if (value) {
        newActs[index].retry_policy = {
          ...newActs[index].retry_policy,
          [field]: field === 'max_attempts' ? parseInt(value) : (field === 'backoff_coefficient' ? parseFloat(value) : value),
          max_attempts: newActs[index].retry_policy?.max_attempts || 3
        };
      } else {
        if (newActs[index].retry_policy) {
          delete (newActs[index].retry_policy as any)[field];
          if (Object.keys(newActs[index].retry_policy as any).length === 0 || (Object.keys(newActs[index].retry_policy as any).length === 1 && (newActs[index].retry_policy as any).max_attempts === 3 && field !== 'max_attempts')) {
             if(field === 'max_attempts') delete newActs[index].retry_policy;
          }
        }
      }
    } else {
      (newActs[index] as any)[field] = value;
    }
    setActivities(newActs);
  };

  const removeActivity = (index: number, e: React.MouseEvent) => {
    e.stopPropagation();
    setActivities(activities.filter((_, i) => i !== index));
    if (expandedActivityIndex === index) {
      setExpandedActivityIndex(null);
    } else if (expandedActivityIndex && expandedActivityIndex > index) {
      setExpandedActivityIndex(expandedActivityIndex - 1);
    }
  };

  return (
    <div>
      <div className="flex justify-between items-center mb-8">
        <h1 className="page-title mb-0">Workflows</h1>
        <div className="flex gap-4">
          <button className="btn btn-secondary px-4 py-2" onClick={seedWorkflows}>
            <Play size={18} className="mr-1 inline" /> Seed Demos
          </button>
          <button className="btn btn-primary px-4 py-2" onClick={handleOpenCreate}>
            <Plus size={18} className="mr-1 inline" /> New Workflow
          </button>
        </div>
      </div>

      {isFormOpen && (
        <div className="card mb-10 shadow-lg border-gray-700">
          <h2 className="text-2xl font-bold mb-6">{editingWorkflowId ? 'Edit Workflow' : 'Create New Workflow'}</h2>
          <form onSubmit={handleSave}>
            <div className="form-group mb-8">
              <label className="form-label text-base">Workflow Name</label>
              <input className="input py-2.5 px-4 text-base" value={newName} onChange={e => setNewName(e.target.value)} required placeholder="e.g. order-processing" />
            </div>
            
            <div className="mb-6">
              <div className="flex justify-between items-center mb-4">
                <label className="form-label text-base mb-0">Activities</label>
                <button type="button" className="btn btn-secondary px-3 py-1.5 text-sm" onClick={addActivity}>
                  <Plus size={14} className="mr-1 inline" /> Add Activity
                </button>
              </div>
              
              {activities.length === 0 && (
                <div className="p-4 border border-dashed border-gray-700 rounded text-center text-gray-500 text-sm">
                  No activities added yet.
                </div>
              )}

              {activities.map((act, i) => (
                <div key={i} className="border border-gray-700/60 rounded-lg mb-3 bg-gray-900/50 overflow-hidden transition-all hover:border-gray-600">
                  <div 
                    className="flex justify-between items-center p-4 cursor-pointer hover:bg-gray-800/50 transition-colors"
                    onClick={() => setExpandedActivityIndex(expandedActivityIndex === i ? null : i)}
                  >
                    <div className="flex items-center gap-3">
                      <div className="text-gray-400 bg-gray-800 p-1 rounded-md">
                        {expandedActivityIndex === i ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                      </div>
                      <h4 className="font-semibold text-sm m-0 text-gray-200">Activity {i + 1}: <span className="text-white">{act.name || 'Unnamed'}</span></h4>
                    </div>
                    <button type="button" onClick={(e) => removeActivity(i, e)} className="text-gray-500 hover:text-red-400 hover:bg-red-400/10 transition-colors p-2 rounded-md">
                      <Trash2 size={16} />
                    </button>
                  </div>
                  
                  {expandedActivityIndex === i && (
                    <div className="p-5 border-t border-gray-700/60 bg-gray-900">
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        <div className="form-group mb-0">
                          <label className="form-label text-sm text-gray-300">Name</label>
                          <input className="input py-2 px-3" value={act.name} onChange={e => updateActivity(i, 'name', e.target.value)} required placeholder="e.g. charge-card" />
                        </div>
                        <div className="form-group mb-0">
                          <label className="form-label text-sm text-gray-300">Timeout</label>
                          <input className="input py-2 px-3" value={act.timeout} onChange={e => updateActivity(i, 'timeout', e.target.value)} required placeholder="e.g. 5m" />
                        </div>
                        <div className="form-group mb-0">
                          <label className="form-label text-sm text-gray-300">Compensation (Optional)</label>
                          <input className="input py-2 px-3" value={act.compensation || ''} onChange={e => updateActivity(i, 'compensation', e.target.value)} placeholder="e.g. refund-card" />
                        </div>
                        <div className="form-group mb-0">
                          <label className="form-label text-sm text-gray-300">Max Retries (Optional)</label>
                          <input type="number" min="0" className="input py-2 px-3" value={act.retry_policy?.max_attempts || ''} onChange={e => updateActivity(i, 'max_attempts', e.target.value)} placeholder="e.g. 3" />
                        </div>
                        <div className="form-group mb-0">
                          <label className="form-label text-sm text-gray-300">Initial Interval (Optional)</label>
                          <input className="input py-2 px-3" value={act.retry_policy?.initial_interval || ''} onChange={e => updateActivity(i, 'initial_interval', e.target.value)} placeholder="e.g. 5s" />
                        </div>
                        <div className="form-group mb-0">
                          <label className="form-label text-sm text-gray-300">Backoff Coeff (Optional)</label>
                          <input type="number" step="0.1" className="input py-2 px-3" value={act.retry_policy?.backoff_coefficient || ''} onChange={e => updateActivity(i, 'backoff_coefficient', e.target.value)} placeholder="e.g. 2.0" />
                        </div>
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>

            <div className="flex gap-4 mt-8 mb-4">
              <button type="submit" className="btn btn-primary px-5 py-2.5">{editingWorkflowId ? 'Save Changes' : 'Create Workflow'}</button>
              <button type="button" className="btn btn-secondary px-5 py-2.5" onClick={() => setIsFormOpen(false)}>Cancel</button>
            </div>
          </form>
        </div>
      )}

      {loading ? (
        <div>Loading workflows...</div>
      ) : (
        <>
          <div className="table-container">
            <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>Created At</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {workflows.map((wf) => (
                <tr key={wf.id}>
                  <td className="font-medium">{wf.name}</td>
                  <td>{new Date(wf.created_at).toLocaleString()}</td>
                  <td>
                    <div className="flex gap-3">
                      <button className="btn btn-primary px-3 py-1.5 text-sm" onClick={() => startWorkflow(wf.id, wf.name)}>
                        <Play size={14} className="mr-1 inline" /> Start
                      </button>
                      <button className="btn btn-secondary px-3 py-1.5 text-sm bg-purple-600 hover:bg-purple-700 text-white border-transparent" onClick={() => batchWorkflow(wf.id, wf.name)}>
                        <Play size={14} className="mr-1 inline" /> Batch (10x)
                      </button>
                      <button className="btn btn-secondary px-3 py-1.5 text-sm" onClick={() => handleOpenEdit(wf.id)}>
                        <Edit2 size={14} className="mr-1 inline" /> Edit
                      </button>
                      <button className="btn btn-danger px-3 py-1.5 text-sm" onClick={() => deleteWorkflow(wf.id, wf.name)}>
                        <Trash2 size={14} className="mr-1 inline" /> Delete
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
              {workflows.length === 0 && (
                <tr>
                  <td colSpan={3} className="text-center py-8 text-gray-500">No workflows found</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
          
        <div className="flex justify-between items-center mt-6 px-1">
            <button 
              className="btn btn-secondary" 
              disabled={page === 0} 
              onClick={() => setPage(p => Math.max(0, p - 1))}
            >
              Previous
            </button>
            <span className="text-sm text-gray-400">Page {page + 1}</span>
            <button 
              className="btn btn-secondary" 
              disabled={workflows.length < pageSize} 
              onClick={() => setPage(p => p + 1)}
            >
              Next
            </button>
          </div>
        </>
      )}
    </div>
  );
};

export default Workflows;
