import { useEffect, useState, useRef } from 'react';
import { Activity, Server, Radio, ShieldAlert, Zap, Box, BrainCircuit, Users } from 'lucide-react';
import './App.css';

interface AgentEvent {
  id: string;
  from: string;
  to: string;
  type: string;
  timestamp: string;
  payload: any;
}

function App() {
  const [events, setEvents] = useState<AgentEvent[]>([]);
  const [status, setStatus] = useState<'connecting' | 'connected' | 'error'>('connecting');
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const es = new EventSource('http://localhost:8080/stream');

    es.onopen = () => setStatus('connected');
    
    es.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data) as AgentEvent;
        // 过滤掉过于频繁的 heartbeat，只展示重要的业务或系统消息
        if (msg.type !== 'heartbeat') {
          setEvents((prev) => {
            const newEvents = [...prev, msg];
            return newEvents.slice(-50); // 保留最近50条
          });
        }
      } catch (err) {
        console.error("Parse event error", err);
      }
    };

    es.onerror = () => setStatus('error');

    return () => es.close();
  }, []);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [events]);

  const getEventIcon = (type: string) => {
    switch (type) {
      case 'system.spawn': return <Zap className="w-5 h-5 text-yellow-400" />;
      case 'system.task_tender': return <Users className="w-5 h-5 text-blue-400" />;
      case 'system.bid_submission': return <BrainCircuit className="w-5 h-5 text-purple-400" />;
      case 'system.task_award': return <Activity className="w-5 h-5 text-green-400" />;
      case 'system.alert': return <ShieldAlert className="w-5 h-5 text-red-500" />;
      default: return <Box className="w-5 h-5 text-gray-400" />;
    }
  };

  const getEventColor = (type: string) => {
    if (type.includes('spawn')) return 'border-yellow-500/30 bg-yellow-500/5 text-yellow-200';
    if (type.includes('tender')) return 'border-blue-500/30 bg-blue-500/5 text-blue-200';
    if (type.includes('bid')) return 'border-purple-500/30 bg-purple-500/5 text-purple-200';
    if (type.includes('alert')) return 'border-red-500/30 bg-red-500/5 text-red-200';
    if (type.includes('award') || type.includes('success')) return 'border-green-500/30 bg-green-500/5 text-green-200';
    return 'border-slate-700 bg-slate-800/50 text-slate-300';
  };

  return (
    <div className="w-full h-screen p-6 flex flex-col items-center">
      {/* Header */}
      <div className="w-full max-w-5xl flex items-center justify-between mb-8 pb-4 border-b border-slate-700/50">
        <div className="flex items-center gap-3">
          <div className="bg-blue-600/20 p-2 rounded-lg border border-blue-500/30">
            <Server className="text-blue-400 w-6 h-6" />
          </div>
          <div>
            <h1 className="text-2xl font-bold bg-gradient-to-r from-blue-400 to-indigo-300 bg-clip-text text-transparent">
              Aether Nexus
            </h1>
            <p className="text-slate-400 text-sm tracking-widest font-mono">
              GLOBAL AGENTIC ORCHESTRATION
            </p>
          </div>
        </div>

        <div className="flex items-center gap-4 bg-slate-800/80 px-4 py-2 rounded-full border border-slate-700">
          <span className="text-slate-400 text-sm">Cluster Status:</span>
          <div className="flex items-center gap-2">
            <div className={`w-3 h-3 rounded-full ${
              status === 'connected' ? 'bg-green-500 status-indicator' :
              status === 'connecting' ? 'bg-yellow-500 animate-pulse' : 'bg-red-500'
            }`} />
            <span className={`text-sm font-bold ${
              status === 'connected' ? 'text-green-400' :
              status === 'connecting' ? 'text-yellow-400' : 'text-red-400'
            }`}>
              {status.toUpperCase()}
            </span>
          </div>
        </div>
      </div>

      {/* Main Content */}
      <div className="w-full max-w-5xl flex-1 flex flex-col overflow-hidden bg-[#0a0f1c]/80 border border-slate-700/50 rounded-xl shadow-2xl shadow-blue-900/10 backdrop-blur-sm">
        
        {/* Panel Header */}
        <div className="flex items-center px-4 py-3 bg-slate-800/50 border-b border-slate-700/50 gap-2">
          <Radio className="text-slate-400 w-4 h-4 animate-pulse" />
          <span className="text-slate-300 font-mono text-sm tracking-wider">LIVE_TELEMETRY_STREAM</span>
        </div>

        {/* Event List */}
        <div 
          ref={scrollRef}
          className="flex-1 overflow-y-auto p-4 space-y-3 terminal-scroll"
        >
          {events.length === 0 ? (
            <div className="h-full flex flex-col items-center justify-center text-slate-500 space-y-4">
              <Activity className="w-12 h-12 opacity-20" />
              <p className="font-mono text-sm">Awaiting cluster events...</p>
            </div>
          ) : (
            events.map((ev, i) => (
              <div 
                key={i} 
                className={`event-enter flex gap-4 p-4 rounded-lg border ${getEventColor(ev.type)} transition-colors duration-300`}
              >
                <div className="pt-1">
                  {getEventIcon(ev.type)}
                </div>
                <div className="flex-1 min-w-0">
                  <div className="flex justify-between items-start mb-2">
                    <div className="flex items-center gap-2">
                      <span className="font-bold text-sm tracking-wide opacity-90">[{ev.type}]</span>
                      <span className="text-xs opacity-60">
                        {ev.from} → {ev.to}
                      </span>
                    </div>
                    <span className="text-xs opacity-50 font-mono">
                      {new Date(ev.timestamp).toLocaleTimeString([], { hour12: false, fractionalSecondDigits: 3 })}
                    </span>
                  </div>
                  
                  <div className="bg-black/30 rounded p-3 font-mono text-xs overflow-x-auto whitespace-pre-wrap border border-white/5 shadow-inner">
                    {JSON.stringify(ev.payload, null, 2)}
                  </div>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  );
}

export default App;
