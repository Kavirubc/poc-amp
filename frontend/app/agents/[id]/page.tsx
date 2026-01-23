'use client';

import { useEffect, useState, useRef, useCallback } from 'react';
import { useParams, useRouter } from 'next/navigation';
import { Agent, getAgent, startAgent, stopAgent, deleteAgent, getLogsUrl } from '@/lib/api';
import StatusBadge from '@/components/StatusBadge';

export default function AgentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const id = params.id as string;

  const [agent, setAgent] = useState<Agent | null>(null);
  const [loading, setLoading] = useState(true);
  const [logs, setLogs] = useState<string[]>([]);
  const [actionLoading, setActionLoading] = useState(false);
  const logsEndRef = useRef<HTMLDivElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  const fetchAgent = useCallback(async () => {
    try {
      const data = await getAgent(id);
      setAgent(data.agent || null);
    } catch (err) {
      console.error('Failed to fetch agent:', err);
    } finally {
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    fetchAgent();
    const interval = setInterval(fetchAgent, 3000);
    return () => clearInterval(interval);
  }, [fetchAgent]);

  useEffect(() => {
    if (!agent || agent.status !== 'running') {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
      return;
    }

    const eventSource = new EventSource(getLogsUrl(id));
    eventSourceRef.current = eventSource;

    eventSource.onmessage = (event) => {
      setLogs(prev => [...prev.slice(-500), event.data]);
    };

    eventSource.onerror = () => {
      eventSource.close();
    };

    return () => {
      eventSource.close();
    };
  }, [agent?.status, id]);

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [logs]);

  const handleStart = async () => {
    setActionLoading(true);
    try {
      await startAgent(id);
      fetchAgent();
    } catch (err) {
      console.error('Failed to start agent:', err);
    } finally {
      setActionLoading(false);
    }
  };

  const handleStop = async () => {
    setActionLoading(true);
    try {
      await stopAgent(id);
      setLogs([]);
      fetchAgent();
    } catch (err) {
      console.error('Failed to stop agent:', err);
    } finally {
      setActionLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm('Are you sure you want to delete this agent?')) return;
    setActionLoading(true);
    try {
      await deleteAgent(id);
      router.push('/');
    } catch (err) {
      console.error('Failed to delete agent:', err);
      setActionLoading(false);
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  if (!agent) {
    return (
      <div className="text-center py-12">
        <h2 className="text-xl font-semibold text-gray-900">Agent not found</h2>
        <a href="/" className="text-blue-600 hover:underline mt-2 inline-block">
          Back to dashboard
        </a>
      </div>
    );
  }

  const isRunning = agent.status === 'running';
  const isStopped = agent.status === 'stopped' || agent.status === 'failed';
  const isProcessing = ['pending', 'cloning', 'building'].includes(agent.status);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold text-gray-900">{agent.name}</h1>
            <StatusBadge status={agent.status} />
          </div>
          <p className="text-gray-600">{agent.type || 'Detecting type...'}</p>
        </div>
        <div className="flex gap-2">
          {isStopped && (
            <button
              onClick={handleStart}
              disabled={actionLoading}
              className="bg-green-600 text-white px-4 py-2 rounded-md hover:bg-green-700 disabled:opacity-50"
            >
              Start
            </button>
          )}
          {isRunning && (
            <button
              onClick={handleStop}
              disabled={actionLoading}
              className="bg-yellow-600 text-white px-4 py-2 rounded-md hover:bg-yellow-700 disabled:opacity-50"
            >
              Stop
            </button>
          )}
          {isProcessing && (
            <button disabled className="bg-gray-400 text-white px-4 py-2 rounded-md cursor-not-allowed">
              Processing...
            </button>
          )}
          <button
            onClick={handleDelete}
            disabled={actionLoading}
            className="bg-red-600 text-white px-4 py-2 rounded-md hover:bg-red-700 disabled:opacity-50"
          >
            Delete
          </button>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <div className="bg-white rounded-lg border border-gray-200 p-6">
          <h2 className="text-lg font-semibold mb-4">Details</h2>
          <dl className="space-y-3">
            <div>
              <dt className="text-sm font-medium text-gray-500">Repository</dt>
              <dd className="text-sm text-gray-900">
                <a href={agent.repo_url} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
                  {agent.repo_url}
                </a>
              </dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Branch</dt>
              <dd className="text-sm text-gray-900">{agent.branch}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Port</dt>
              <dd className="text-sm text-gray-900">
                {agent.port > 0 && isRunning ? (
                  <a href={`http://localhost:${agent.port}`} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
                    {agent.port}
                  </a>
                ) : (
                  agent.port || 'Not assigned'
                )}
              </dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Container ID</dt>
              <dd className="text-sm text-gray-900 font-mono">{agent.container_id || 'None'}</dd>
            </div>
            <div>
              <dt className="text-sm font-medium text-gray-500">Created</dt>
              <dd className="text-sm text-gray-900">{new Date(agent.created_at).toLocaleString()}</dd>
            </div>
            {agent.error && (
              <div>
                <dt className="text-sm font-medium text-red-500">Error</dt>
                <dd className="text-sm text-red-700">{agent.error}</dd>
              </div>
            )}
          </dl>
        </div>

        <div className="bg-white rounded-lg border border-gray-200 p-6">
          <h2 className="text-lg font-semibold mb-4">Environment Variables</h2>
          {agent.env_content ? (
            <pre className="bg-gray-900 text-green-400 p-4 rounded-lg text-sm overflow-x-auto font-mono">
              {agent.env_content}
            </pre>
          ) : (
            <p className="text-gray-500">No environment variables configured</p>
          )}
        </div>
      </div>

      <div className="mt-6 bg-white rounded-lg border border-gray-200 p-6">
        <h2 className="text-lg font-semibold mb-4">Logs</h2>
        <div className="bg-gray-900 text-gray-100 p-4 rounded-lg h-96 overflow-y-auto font-mono text-sm">
          {logs.length === 0 ? (
            <p className="text-gray-500">
              {isRunning ? 'Waiting for logs...' : 'Start the agent to view logs'}
            </p>
          ) : (
            logs.map((log, index) => (
              <div key={index} className="whitespace-pre-wrap">
                {log}
              </div>
            ))
          )}
          <div ref={logsEndRef} />
        </div>
      </div>
    </div>
  );
}
