import { startTransition, useEffect, useState, type FormEvent } from 'react';
import { Activity, ChevronRight, Clock3, FileText, ListTodo, RotateCcw, Server, ShieldCheck, Sparkles, Square, TerminalSquare, Wifi } from 'lucide-react';
import './App.css';

type TaskStatus = 'queued' | 'running' | 'reviewing' | 'completed' | 'failed' | 'cancelled';

interface Task {
  id: string;
  source: string;
  mode: string;
  description: string;
  input?: Record<string, unknown>;
  status: TaskStatus;
  trace_id?: string;
  current_stage?: string;
  final_output?: string;
  error_summary?: string;
  created_at: string;
  updated_at: string;
}

interface TaskEvent {
  id: number;
  task_id: string;
  message_id?: string;
  from?: string;
  to?: string;
  type: string;
  payload?: Record<string, unknown>;
  created_at: string;
}

interface AgentStats {
  active_agents: number;
  total_spawns: number;
  total_failures: number;
  status_counts?: Record<string, number>;
}

interface HealthSnapshot {
  status: string;
  timestamp: string;
  components: Record<string, string>;
  agent_stats: AgentStats;
}

interface AgentSnapshot {
  name: string;
  role: string;
  status: string;
  metadata?: Record<string, unknown>;
}

interface AgentsResponse {
  items: AgentSnapshot[];
  stats: AgentStats;
}

interface TaskStreamSnapshot {
  task: Task;
  events: TaskEvent[];
}

interface TaskStreamUpdate {
  task?: Task;
  event?: TaskEvent;
}

function upsertTask(currentTasks: Task[], nextTask: Task) {
  const existingIndex = currentTasks.findIndex((task) => task.id === nextTask.id);
  if (existingIndex === -1) {
    return [nextTask, ...currentTasks].slice(0, 50);
  }

  const nextTasks = [...currentTasks];
  nextTasks[existingIndex] = nextTask;
  return nextTasks;
}

function appendEvent(currentEvents: TaskEvent[], nextEvent: TaskEvent) {
  if (currentEvents.some((eventItem) => eventItem.id === nextEvent.id)) {
    return currentEvents;
  }
  return [...currentEvents, nextEvent];
}

function App() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [selectedTaskId, setSelectedTaskId] = useState('');
  const [selectedTask, setSelectedTask] = useState<Task | null>(null);
  const [events, setEvents] = useState<TaskEvent[]>([]);
  const [status, setStatus] = useState<'loading' | 'connected' | 'error'>('loading');
  const [streamStatus, setStreamStatus] = useState<'idle' | 'live' | 'error'>('idle');
  const [draft, setDraft] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState('');
  const [actionState, setActionState] = useState<'idle' | 'retry' | 'cancel'>('idle');
  const [actionError, setActionError] = useState('');
  const [health, setHealth] = useState<HealthSnapshot | null>(null);
  const [agents, setAgents] = useState<AgentSnapshot[]>([]);
  const [agentStats, setAgentStats] = useState<AgentStats | null>(null);
  const daemonBaseUrl = import.meta.env.VITE_AETHERD_URL ?? 'http://localhost:8080';

  async function loadTasks() {
    try {
      const response = await fetch(`${daemonBaseUrl}/api/v1/tasks?limit=50`);
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const nextTasks = (await response.json()) as Task[];
      startTransition(() => {
        setTasks(nextTasks);
        setStatus('connected');

        if (nextTasks.length === 0) {
          setSelectedTaskId('');
          setSelectedTask(null);
          setEvents([]);
          return;
        }

        if (!selectedTaskId) {
          setSelectedTaskId(nextTasks[0].id);
          return;
        }

        const stillExists = nextTasks.some((task) => task.id === selectedTaskId);
        if (!stillExists) {
          setSelectedTaskId(nextTasks[0].id);
        }
      });
    } catch (err) {
      console.error('Load tasks failed', err);
      setStatus('error');
    }
  }

  async function loadSelectedTask(taskID: string) {
    try {
      const [taskResponse, eventsResponse] = await Promise.all([
        fetch(`${daemonBaseUrl}/api/v1/tasks/${taskID}`),
        fetch(`${daemonBaseUrl}/api/v1/tasks/${taskID}/events`),
      ]);

      if (!taskResponse.ok || !eventsResponse.ok) {
        throw new Error('Task detail fetch failed');
      }

      const [task, taskEvents] = await Promise.all([
        taskResponse.json() as Promise<Task>,
        eventsResponse.json() as Promise<TaskEvent[]>,
      ]);

      startTransition(() => {
        setSelectedTask(task);
        setEvents(taskEvents);
        setStatus('connected');
      });
    } catch (err) {
      console.error('Load selected task failed', err);
      setStatus('error');
    }
  }

  async function loadSystemState() {
    try {
      const [healthResponse, agentsResponse] = await Promise.all([
        fetch(`${daemonBaseUrl}/api/v1/health`),
        fetch(`${daemonBaseUrl}/api/v1/agents`),
      ]);

      if (!healthResponse.ok || !agentsResponse.ok) {
        throw new Error('System state fetch failed');
      }

      const [healthPayload, agentsPayload] = await Promise.all([
        healthResponse.json() as Promise<HealthSnapshot>,
        agentsResponse.json() as Promise<AgentsResponse>,
      ]);

      startTransition(() => {
        setHealth(healthPayload);
        setAgents(agentsPayload.items);
        setAgentStats(agentsPayload.stats);
      });
    } catch (err) {
      console.error('Load system state failed', err);
    }
  }

  useEffect(() => {
    void loadTasks();
    const interval = window.setInterval(() => {
      void loadTasks();
    }, 5000);
    return () => window.clearInterval(interval);
  }, [daemonBaseUrl, selectedTaskId]);

  useEffect(() => {
    void loadSystemState();
    const interval = window.setInterval(() => {
      void loadSystemState();
    }, 10000);
    return () => window.clearInterval(interval);
  }, [daemonBaseUrl]);

  useEffect(() => {
    if (!selectedTaskId) {
      setStreamStatus('idle');
      return;
    }

    void loadSelectedTask(selectedTaskId);

    const stream = new EventSource(`${daemonBaseUrl}/api/v1/tasks/${selectedTaskId}/events/stream`);

    const handleSnapshot = (event: Event) => {
      const snapshot = JSON.parse((event as MessageEvent<string>).data) as TaskStreamSnapshot;
      startTransition(() => {
        setSelectedTask(snapshot.task);
        setEvents(snapshot.events);
        setTasks((currentTasks) => upsertTask(currentTasks, snapshot.task));
        setStatus('connected');
        setStreamStatus('live');
      });
    };

    const handleUpdate = (event: Event) => {
      const update = JSON.parse((event as MessageEvent<string>).data) as TaskStreamUpdate;
      startTransition(() => {
        if (update.task) {
          setSelectedTask(update.task);
          setTasks((currentTasks) => upsertTask(currentTasks, update.task as Task));
        }
        if (update.event) {
          setEvents((currentEvents) => appendEvent(currentEvents, update.event as TaskEvent));
        }
        setStreamStatus('live');
      });
    };

    stream.addEventListener('snapshot', handleSnapshot);
    stream.addEventListener('update', handleUpdate);
    stream.onopen = () => {
      setStreamStatus('live');
    };
    stream.onerror = () => {
      setStreamStatus('error');
    };

    return () => {
      stream.removeEventListener('snapshot', handleSnapshot);
      stream.removeEventListener('update', handleUpdate);
      stream.close();
      setStreamStatus('idle');
    };
  }, [daemonBaseUrl, selectedTaskId]);

  async function createTask(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!draft.trim()) {
      setSubmitError('Task description is required.');
      return;
    }

    setSubmitting(true);
    setSubmitError('');

    try {
      const response = await fetch(`${daemonBaseUrl}/api/v1/tasks`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          source: 'web_ui',
          mode: 'agent',
          description: draft.trim(),
        }),
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const createdTask = (await response.json()) as Task;
      startTransition(() => {
        setDraft('');
        setSelectedTaskId(createdTask.id);
        setSelectedTask(createdTask);
        setEvents([]);
        setActionError('');
      });
      await loadTasks();
    } catch (err) {
      console.error('Create task failed', err);
      setSubmitError('Failed to create task.');
    } finally {
      setSubmitting(false);
    }
  }

  async function handleTaskAction(action: 'retry' | 'cancel') {
    if (!selectedTask) {
      return;
    }

    setActionState(action);
    setActionError('');

    const endpoint = action === 'retry'
      ? `${daemonBaseUrl}/api/v1/tasks/${selectedTask.id}/retry`
      : `${daemonBaseUrl}/api/v1/tasks/${selectedTask.id}/cancel`;

    try {
      const response = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: action === 'cancel'
          ? JSON.stringify({ reason: 'Cancelled from control plane UI.' })
          : undefined,
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const taskResponse = (await response.json()) as Task;
      startTransition(() => {
        if (action === 'retry') {
          setSelectedTaskId(taskResponse.id);
          setSelectedTask(taskResponse);
          setEvents([]);
        } else {
          setSelectedTask(taskResponse);
          setTasks((currentTasks) => upsertTask(currentTasks, taskResponse));
        }
      });

      await loadTasks();
      if (action === 'cancel') {
        await loadSelectedTask(selectedTask.id);
      }
    } catch (err) {
      console.error(`Task ${action} failed`, err);
      setActionError(`Failed to ${action} task.`);
    } finally {
      setActionState('idle');
    }
  }

  function getStatusTone(statusValue: TaskStatus) {
    switch (statusValue) {
      case 'completed':
        return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-200';
      case 'failed':
        return 'border-rose-500/30 bg-rose-500/10 text-rose-200';
      case 'reviewing':
        return 'border-amber-500/30 bg-amber-500/10 text-amber-200';
      case 'running':
        return 'border-sky-500/30 bg-sky-500/10 text-sky-200';
      case 'cancelled':
        return 'border-slate-500/30 bg-slate-500/10 text-slate-300';
      default:
        return 'border-indigo-500/30 bg-indigo-500/10 text-indigo-200';
    }
  }

  const canCancel = selectedTask ? ['queued', 'running', 'reviewing'].includes(selectedTask.status) : false;
  const canRetry = selectedTask ? ['completed', 'failed', 'cancelled'].includes(selectedTask.status) : false;

  return (
    <div className="w-full min-h-screen px-4 py-6 md:px-6">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-6">
        <div className="flex items-center gap-3">
          <div className="rounded-lg border border-blue-500/30 bg-blue-600/20 p-2">
            <Server className="h-6 w-6 text-blue-400" />
          </div>
          <div>
            <h1 className="bg-gradient-to-r from-blue-300 via-cyan-200 to-emerald-200 bg-clip-text text-3xl font-bold text-transparent">
              Aether Control Plane
            </h1>
            <p className="font-mono text-sm tracking-widest text-slate-400">
              TASK LIFECYCLE, LIVE STATUS, FINAL REPORTS
            </p>
          </div>
        </div>

        <div className="grid gap-6 lg:grid-cols-[360px_minmax(0,1fr)]">
          <aside className="overflow-hidden rounded-3xl border border-slate-700/50 bg-[#0a0f1c]/85 shadow-2xl shadow-cyan-950/20 backdrop-blur">
            <div className="border-b border-slate-700/50 px-5 py-4">
              <div className="mb-4 flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <ListTodo className="h-4 w-4 text-cyan-300" />
                  <span className="font-mono text-sm tracking-[0.2em] text-slate-300">TASKS</span>
                </div>
                <span className={`rounded-full border px-3 py-1 text-xs font-semibold ${status === 'connected' ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-300' : status === 'loading' ? 'border-amber-500/30 bg-amber-500/10 text-amber-300' : 'border-rose-500/30 bg-rose-500/10 text-rose-300'}`}>
                  {status.toUpperCase()}
                </span>
              </div>

              <form className="space-y-3" onSubmit={createTask}>
                <label className="block text-xs font-semibold uppercase tracking-[0.2em] text-slate-400">
                  New Task
                </label>
                <textarea
                  className="min-h-28 w-full rounded-2xl border border-slate-700 bg-slate-950/70 px-4 py-3 text-sm text-slate-100 outline-none transition focus:border-cyan-400/50"
                  placeholder="Describe the task you want Aether to run..."
                  value={draft}
                  onChange={(event) => setDraft(event.target.value)}
                />
                <button
                  type="submit"
                  disabled={submitting}
                  className="flex w-full items-center justify-center gap-2 rounded-2xl border border-cyan-500/30 bg-cyan-500/10 px-4 py-3 text-sm font-semibold text-cyan-100 transition hover:bg-cyan-500/20 disabled:cursor-not-allowed disabled:opacity-60"
                >
                  <Sparkles className="h-4 w-4" />
                  {submitting ? 'Submitting...' : 'Create Task'}
                </button>
                {submitError ? (
                  <p className="text-sm text-rose-300">{submitError}</p>
                ) : null}
              </form>
            </div>

            <div className="max-h-[60vh] overflow-y-auto p-3 terminal-scroll">
              {tasks.length === 0 ? (
                <div className="flex min-h-60 flex-col items-center justify-center gap-3 rounded-2xl border border-dashed border-slate-700 bg-slate-900/30 p-6 text-center text-slate-500">
                  <Activity className="h-10 w-10 opacity-30" />
                  <p className="font-mono text-sm">No tasks yet. Submit one to start the pipeline.</p>
                </div>
              ) : (
                <div className="space-y-3">
                  {tasks.map((task) => {
                    const isSelected = task.id === selectedTaskId;
                    return (
                      <button
                        key={task.id}
                        type="button"
                        onClick={() => setSelectedTaskId(task.id)}
                        className={`w-full rounded-2xl border p-4 text-left transition ${isSelected ? 'border-cyan-400/40 bg-cyan-500/10 shadow-lg shadow-cyan-950/20' : 'border-slate-700 bg-slate-900/50 hover:border-slate-600 hover:bg-slate-900/80'}`}
                      >
                        <div className="mb-3 flex items-start justify-between gap-3">
                          <span className={`rounded-full border px-2.5 py-1 text-xs font-semibold uppercase ${getStatusTone(task.status)}`}>
                            {task.status}
                          </span>
                          <ChevronRight className={`h-4 w-4 flex-none transition ${isSelected ? 'text-cyan-300' : 'text-slate-500'}`} />
                        </div>
                        <p className="line-clamp-3 text-sm leading-6 text-slate-100">
                          {task.description}
                        </p>
                        <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-slate-400">
                          <div>
                            <span className="block uppercase tracking-[0.2em] text-slate-500">Source</span>
                            <span className="font-mono">{task.source}</span>
                          </div>
                          <div>
                            <span className="block uppercase tracking-[0.2em] text-slate-500">Stage</span>
                            <span className="font-mono">{task.current_stage || '-'}</span>
                          </div>
                        </div>
                      </button>
                    );
                  })}
                </div>
              )}
            </div>
          </aside>

          <main className="overflow-hidden rounded-3xl border border-slate-700/50 bg-[#09121f]/90 shadow-2xl shadow-blue-950/20 backdrop-blur">
            <div className="border-b border-slate-700/50 px-6 py-5">
              <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                <div className="space-y-2">
                  <div className="flex items-center gap-2 text-slate-400">
                    <TerminalSquare className="h-4 w-4" />
                    <span className="font-mono text-sm tracking-[0.2em]">TASK DETAIL</span>
                  </div>
                  <h2 className="text-xl font-semibold text-slate-100">
                    {selectedTask ? selectedTask.description : 'Select a task'}
                  </h2>
                  <p className="font-mono text-xs text-slate-500">
                    {selectedTask ? selectedTask.id : 'No task selected'}
                  </p>
                </div>

                {selectedTask ? (
                  <div className="flex flex-wrap items-center justify-end gap-3">
                    <button
                      type="button"
                      disabled={!canRetry || actionState !== 'idle'}
                      onClick={() => void handleTaskAction('retry')}
                      className="flex items-center gap-2 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-3 py-1.5 text-sm font-semibold text-emerald-100 transition hover:bg-emerald-500/20 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <RotateCcw className="h-4 w-4" />
                      {actionState === 'retry' ? 'Retrying...' : 'Retry'}
                    </button>
                    <button
                      type="button"
                      disabled={!canCancel || actionState !== 'idle'}
                      onClick={() => void handleTaskAction('cancel')}
                      className="flex items-center gap-2 rounded-full border border-rose-500/30 bg-rose-500/10 px-3 py-1.5 text-sm font-semibold text-rose-100 transition hover:bg-rose-500/20 disabled:cursor-not-allowed disabled:opacity-50"
                    >
                      <Square className="h-3.5 w-3.5" />
                      {actionState === 'cancel' ? 'Cancelling...' : 'Cancel'}
                    </button>
                    <div className={`self-start rounded-full border px-3 py-1.5 text-sm font-semibold uppercase ${getStatusTone(selectedTask.status)}`}>
                      {selectedTask.status}
                    </div>
                  </div>
                ) : null}
              </div>
              {actionError ? (
                <p className="mt-3 text-sm text-rose-300">{actionError}</p>
              ) : null}
            </div>

            {!selectedTask ? (
              <div className="flex min-h-[70vh] flex-col items-center justify-center gap-4 text-slate-500">
                <FileText className="h-12 w-12 opacity-20" />
                <p className="font-mono text-sm">Pick a task from the left rail to inspect its lifecycle.</p>
              </div>
            ) : (
              <div className="grid gap-6 p-6 xl:grid-cols-[minmax(0,1fr)_360px]">
                <section className="space-y-6">
                  <div className="grid gap-4 md:grid-cols-3">
                    <div className="rounded-2xl border border-slate-700 bg-slate-900/50 p-4">
                      <p className="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">Current Stage</p>
                      <p className="font-mono text-sm text-cyan-200">{selectedTask.current_stage || '-'}</p>
                    </div>
                    <div className="rounded-2xl border border-slate-700 bg-slate-900/50 p-4">
                      <p className="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">Source</p>
                      <p className="font-mono text-sm text-slate-200">{selectedTask.source}</p>
                    </div>
                    <div className="rounded-2xl border border-slate-700 bg-slate-900/50 p-4">
                      <p className="mb-2 text-xs uppercase tracking-[0.2em] text-slate-500">Created</p>
                      <p className="font-mono text-sm text-slate-200">{new Date(selectedTask.created_at).toLocaleString()}</p>
                    </div>
                  </div>

                  <div className="rounded-3xl border border-slate-700 bg-slate-950/60">
                    <div className="border-b border-slate-700/70 px-5 py-4">
                      <span className="font-mono text-sm tracking-[0.2em] text-slate-300">FINAL REPORT</span>
                    </div>
                    <div className="p-5">
                      {selectedTask.final_output ? (
                        <pre className="overflow-x-auto whitespace-pre-wrap rounded-2xl border border-white/5 bg-black/40 p-4 text-sm leading-6 text-slate-100">
                          {selectedTask.final_output}
                        </pre>
                      ) : (
                        <p className="text-sm text-slate-500">No final output yet.</p>
                      )}
                    </div>
                  </div>

                  <div className="rounded-3xl border border-slate-700 bg-slate-950/60">
                    <div className="border-b border-slate-700/70 px-5 py-4">
                      <span className="font-mono text-sm tracking-[0.2em] text-slate-300">EVENT TIMELINE</span>
                    </div>
                    <div className="max-h-[420px] space-y-3 overflow-y-auto p-5 terminal-scroll">
                      {events.length === 0 ? (
                        <p className="text-sm text-slate-500">No persisted events yet.</p>
                      ) : (
                        events.map((eventItem) => (
                          <div key={eventItem.id} className="rounded-2xl border border-slate-700 bg-slate-900/60 p-4">
                            <div className="mb-2 flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
                              <div className="flex items-center gap-2 text-slate-200">
                                <Activity className="h-4 w-4 text-cyan-300" />
                                <span className="font-semibold">{eventItem.type}</span>
                                <span className="text-xs text-slate-500">
                                  {eventItem.from || '-'} → {eventItem.to || '-'}
                                </span>
                              </div>
                              <span className="font-mono text-xs text-slate-500">
                                {new Date(eventItem.created_at).toLocaleTimeString()}
                              </span>
                            </div>
                            {eventItem.payload ? (
                              <pre className="overflow-x-auto whitespace-pre-wrap rounded-xl border border-white/5 bg-black/30 p-3 text-xs text-slate-300">
                                {JSON.stringify(eventItem.payload, null, 2)}
                              </pre>
                            ) : null}
                          </div>
                        ))
                      )}
                    </div>
                  </div>
                </section>

                <aside className="space-y-6">
                  <div className="rounded-3xl border border-slate-700 bg-slate-900/50 p-5">
                    <div className="mb-4 flex items-center gap-2 text-slate-300">
                      <Clock3 className="h-4 w-4 text-cyan-300" />
                      <span className="font-mono text-sm tracking-[0.2em]">META</span>
                    </div>
                    <div className="space-y-4 text-sm">
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.2em] text-slate-500">Mode</p>
                        <p className="font-mono text-slate-200">{selectedTask.mode}</p>
                      </div>
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.2em] text-slate-500">Updated</p>
                        <p className="font-mono text-slate-200">{new Date(selectedTask.updated_at).toLocaleString()}</p>
                      </div>
                      <div>
                        <p className="mb-1 text-xs uppercase tracking-[0.2em] text-slate-500">Trace ID</p>
                        <p className="break-all font-mono text-xs text-slate-300">
                          {selectedTask.trace_id || 'Not recorded yet'}
                        </p>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-3xl border border-slate-700 bg-slate-900/50 p-5">
                    <div className="mb-4 flex items-center gap-2 text-slate-300">
                      <FileText className="h-4 w-4 text-rose-300" />
                      <span className="font-mono text-sm tracking-[0.2em]">ERROR SUMMARY</span>
                    </div>
                    <p className="text-sm leading-6 text-slate-300">
                      {selectedTask.error_summary || 'No error summary.'}
                    </p>
                  </div>

                  <div className="rounded-3xl border border-slate-700 bg-slate-900/50 p-5">
                    <div className="mb-4 flex items-center gap-2 text-slate-300">
                      <ShieldCheck className="h-4 w-4 text-emerald-300" />
                      <span className="font-mono text-sm tracking-[0.2em]">SYSTEM SNAPSHOT</span>
                    </div>
                    <div className="space-y-3 text-sm text-slate-300">
                      <div className="flex items-center justify-between">
                        <span>Daemon Health</span>
                        <span className={`font-mono ${health?.status === 'ok' ? 'text-emerald-300' : 'text-amber-300'}`}>
                          {health?.status ?? 'loading'}
                        </span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span>Task Stream</span>
                        <span className={`font-mono ${streamStatus === 'live' ? 'text-emerald-300' : streamStatus === 'error' ? 'text-rose-300' : 'text-slate-400'}`}>
                          {streamStatus}
                        </span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span>Active Agents</span>
                        <span className="font-mono text-slate-100">{agentStats?.active_agents ?? 0}</span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span>Total Spawns</span>
                        <span className="font-mono text-slate-100">{agentStats?.total_spawns ?? 0}</span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span>Total Tasks Loaded</span>
                        <span className="font-mono text-slate-100">{tasks.length}</span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span>Selected Events</span>
                        <span className="font-mono text-slate-100">{events.length}</span>
                      </div>
                      <div className="flex items-center justify-between">
                        <span>API Status</span>
                        <span className={`font-mono ${status === 'connected' ? 'text-emerald-300' : status === 'loading' ? 'text-amber-300' : 'text-rose-300'}`}>
                          {status}
                        </span>
                      </div>
                    </div>
                    {health ? (
                      <div className="mt-5 space-y-2">
                        {Object.entries(health.components).map(([componentName, componentState]) => (
                          <div key={componentName} className="flex items-center justify-between rounded-2xl border border-slate-700 bg-slate-950/50 px-3 py-2 text-xs text-slate-300">
                            <span className="uppercase tracking-[0.2em] text-slate-500">{componentName}</span>
                            <span className={componentState === 'ready' ? 'text-emerald-300' : 'text-amber-300'}>
                              {componentState}
                            </span>
                          </div>
                        ))}
                      </div>
                    ) : null}
                  </div>

                  <div className="rounded-3xl border border-slate-700 bg-slate-900/50 p-5">
                    <div className="mb-4 flex items-center gap-2 text-slate-300">
                      <Wifi className="h-4 w-4 text-cyan-300" />
                      <span className="font-mono text-sm tracking-[0.2em]">AGENT ROSTER</span>
                    </div>
                    {agents.length === 0 ? (
                      <p className="text-sm text-slate-500">No active agents reported by the daemon.</p>
                    ) : (
                      <div className="space-y-3">
                        {agents.slice(0, 6).map((agentItem) => (
                          <div key={agentItem.name} className="rounded-2xl border border-slate-700 bg-slate-950/50 p-3">
                            <div className="mb-2 flex items-center justify-between gap-3">
                              <span className="font-mono text-sm text-slate-100">{agentItem.name}</span>
                              <span className="rounded-full border border-cyan-500/30 bg-cyan-500/10 px-2 py-0.5 text-xs uppercase text-cyan-200">
                                {agentItem.status}
                              </span>
                            </div>
                            <p className="text-xs uppercase tracking-[0.2em] text-slate-500">{agentItem.role}</p>
                            <p className="mt-2 break-all font-mono text-xs text-slate-400">
                              {typeof agentItem.metadata?.task_id === 'string' ? `task: ${agentItem.metadata.task_id}` : 'task: -'}
                            </p>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </aside>
              </div>
            )}
          </main>
        </div>
      </div>
    </div>
  );
}

export default App;
