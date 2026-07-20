import { useState, useEffect } from 'react';
import { useParams, Link } from 'react-router-dom';
import { ArrowLeft, CheckCircle, XCircle, Clock, RotateCcw, AlertTriangle, PlayCircle } from 'lucide-react';
import api from '../api';

const ExecutionDetail = () => {
  const { id } = useParams<{ id: string }>();
  const [execution, setExecution] = useState<any>(null);
  const [events, setEvents] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchDetail = async () => {
    try {
      const [execRes, eventsRes] = await Promise.all([
        api.get(`/executions/${id}`),
        api.get(`/executions/${id}/events`)
      ]);
      setExecution(execRes.data);
      setEvents(eventsRes.data || []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchDetail();
    const interval = setInterval(fetchDetail, 3000);
    return () => clearInterval(interval);
  }, [id]);

  const getEventIcon = (eventType: string) => {
    switch (eventType) {
      case 'WORKFLOW_STARTED':
      case 'ACTIVITY_STARTED':
      case 'COMPENSATION_STARTED':
        return <PlayCircle size={14} className="text-info-color" />;
      case 'ACTIVITY_COMPLETED':
      case 'COMPENSATION_COMPLETED':
      case 'WORKFLOW_COMPLETED':
        return <CheckCircle size={14} className="text-success-color" />;
      case 'WORKFLOW_FAILED':
      case 'ACTIVITY_FAILED':
      case 'COMPENSATION_FAILED':
        return <XCircle size={14} className="text-danger-color" />;
      case 'ACTIVITY_SCHEDULED':
      case 'COMPENSATION_SCHEDULED':
        return <Clock size={14} className="text-warning-color" />;
      case 'ACTIVITY_RETRIED':
        return <RotateCcw size={14} className="text-info-color" />;
      default:
        return <AlertTriangle size={14} />;
    }
  };

  const getEventBadgeClass = (eventType: string) => {
    switch (eventType) {
      case 'WORKFLOW_STARTED':
      case 'ACTIVITY_STARTED':
      case 'COMPENSATION_STARTED':
      case 'ACTIVITY_RETRIED':
      case 'ACTIVITY_SCHEDULED':
      case 'COMPENSATION_SCHEDULED':
        return 'status-pending';
      case 'ACTIVITY_COMPLETED':
      case 'COMPENSATION_COMPLETED':
      case 'WORKFLOW_COMPLETED':
        return 'status-success';
      case 'WORKFLOW_FAILED':
      case 'ACTIVITY_FAILED':
      case 'COMPENSATION_FAILED':
        return 'status-failed';
      default:
        return 'status-pending';
    }
  };

  const getExecutionBadgeClass = (status: string) => {
    switch (status) {
      case 'COMPLETED':
        return 'status-success';
      case 'FAILED':
        return 'status-failed';
      default:
        return 'status-pending';
    }
  };

  if (loading) return <div>Loading...</div>;
  if (!execution) return <div>Execution not found</div>;

  return (
    <div>
      <div className="mb-6">
        <Link to="/executions" className="text-blue-400 hover:text-blue-300 flex items-center gap-2 mb-4">
          <ArrowLeft size={16} /> Back to Executions
        </Link>
        <div className="flex justify-between items-center">
          <h1 className="page-title mb-0">Execution: {id}</h1>
          <span className={`status-badge px-4 py-2 text-sm ${getExecutionBadgeClass(execution.status)}`}>{execution.status}</span>
        </div>
        <div className="text-gray-400 mt-2">Workflow: <span className="font-bold text-white">{execution.workflow_name}</span></div>
      </div>

      <div className="card">
        <h2 className="text-xl font-bold mb-6">Event Timeline</h2>
        {events.length === 0 ? (
          <div className="text-gray-500">No events recorded.</div>
        ) : (
          <div className="timeline">
            {events.map((ev, i) => (
              <div className="timeline-item" key={ev.id || i}>
                <div className="timeline-icon">
                  {getEventIcon(ev.event_type)}
                </div>
                <div className="timeline-content">
                  <div className="flex justify-between items-start mb-2">
                    <div className={`status-badge ${getEventBadgeClass(ev.event_type)}`}>{ev.event_type}</div>
                    <div className="timeline-time text-sm text-gray-500">{new Date(ev.timestamp).toLocaleString()}</div>
                  </div>
                  {ev.payload && Object.keys(ev.payload).length > 0 && (
                    <pre className="timeline-payload bg-gray-900 p-3 rounded border border-gray-800 text-xs text-gray-300 overflow-x-auto mt-2">
                      {JSON.stringify(ev.payload, null, 2)}
                    </pre>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

export default ExecutionDetail;
