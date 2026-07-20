import { useState, useEffect } from 'react';
import api from '../api';
import { Settings, ServerCrash, Server } from 'lucide-react';

export const DemoControls = () => {
  const [isDown, setIsDown] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetchStatus();
  }, []);

  const fetchStatus = async () => {
    try {
      const res = await api.get('/demo/shipment-status');
      setIsDown(res.data.down);
    } catch (e) {
      console.error('Failed to fetch demo status', e);
    } finally {
      setLoading(false);
    }
  };

  const toggleStatus = async () => {
    const newState = !isDown;
    setIsDown(newState); // optimistic update
    try {
      await api.post('/demo/shipment-status', { down: newState });
    } catch (e) {
      console.error('Failed to update demo status', e);
      setIsDown(isDown); // revert
    }
  };

  if (loading) return null;

  return (
    <div className="flex items-center gap-4 bg-gray-800/40 px-5 py-3 rounded-xl border border-gray-700/60 shadow-sm mt-2">
      <span className="text-sm font-semibold text-gray-300">Shipment API:</span>
      <button 
        onClick={toggleStatus}
        className={`flex items-center gap-2 px-4 py-1.5 rounded-lg text-sm font-bold transition-colors ${
          isDown 
            ? 'bg-red-500/20 text-red-400 border border-red-500/50 hover:bg-red-500/30' 
            : 'bg-green-500/20 text-green-400 border border-green-500/50 hover:bg-green-500/30'
        }`}
      >
        {isDown ? <ServerCrash size={14} /> : <Server size={14} />}
        {isDown ? 'DOWN' : 'UP'}
      </button>
    </div>
  );
};
