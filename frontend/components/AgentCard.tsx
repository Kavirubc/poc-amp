'use client';

import Link from 'next/link';
import { Agent, startAgent, stopAgent, deleteAgent } from '@/lib/api';
import StatusBadge from './StatusBadge';
import { useState } from 'react';

interface AgentCardProps {
  agent: Agent;
  onUpdate: () => void;
}

export default function AgentCard({ agent, onUpdate }: AgentCardProps) {
  const [loading, setLoading] = useState(false);

  const handleStart = async () => {
    setLoading(true);
    try {
      await startAgent(agent.id);
      onUpdate();
    } catch (err) {
      console.error('Failed to start agent:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleStop = async () => {
    setLoading(true);
    try {
      await stopAgent(agent.id);
      onUpdate();
    } catch (err) {
      console.error('Failed to stop agent:', err);
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async () => {
    if (!confirm('Are you sure you want to delete this agent?')) return;
    setLoading(true);
    try {
      await deleteAgent(agent.id);
      onUpdate();
    } catch (err) {
      console.error('Failed to delete agent:', err);
    } finally {
      setLoading(false);
    }
  };

  const isRunning = agent.status === 'running';
  const isStopped = agent.status === 'stopped' || agent.status === 'failed';
  const isProcessing = ['pending', 'cloning', 'building'].includes(agent.status);

  return (
    <div className="bg-white rounded-lg shadow border border-gray-200 p-6">
      <div className="flex items-start justify-between mb-4">
        <div>
          <Link href={`/agents/${agent.id}`} className="text-lg font-semibold text-gray-900 hover:text-blue-600">
            {agent.name}
          </Link>
          <p className="text-sm text-gray-500 mt-1">{agent.type || 'detecting...'}</p>
        </div>
        <StatusBadge status={agent.status} />
      </div>

      <div className="space-y-2 text-sm text-gray-600 mb-4">
        <p className="truncate">
          <span className="font-medium">Repo:</span>{' '}
          <a href={agent.repo_url} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
            {agent.repo_url}
          </a>
        </p>
        <p>
          <span className="font-medium">Branch:</span> {agent.branch}
        </p>
        {agent.port > 0 && (
          <p>
            <span className="font-medium">Port:</span>{' '}
            {isRunning ? (
              <a href={`http://localhost:${agent.port}`} target="_blank" rel="noopener noreferrer" className="text-blue-600 hover:underline">
                {agent.port}
              </a>
            ) : (
              agent.port
            )}
          </p>
        )}
        {agent.error && (
          <p className="text-red-600">
            <span className="font-medium">Error:</span> {agent.error}
          </p>
        )}
      </div>

      <div className="flex gap-2">
        {isStopped && (
          <button
            onClick={handleStart}
            disabled={loading}
            className="flex-1 bg-green-600 text-white px-3 py-1.5 rounded text-sm hover:bg-green-700 disabled:opacity-50"
          >
            Start
          </button>
        )}
        {isRunning && (
          <button
            onClick={handleStop}
            disabled={loading}
            className="flex-1 bg-yellow-600 text-white px-3 py-1.5 rounded text-sm hover:bg-yellow-700 disabled:opacity-50"
          >
            Stop
          </button>
        )}
        {isProcessing && (
          <button disabled className="flex-1 bg-gray-400 text-white px-3 py-1.5 rounded text-sm cursor-not-allowed">
            Processing...
          </button>
        )}
        <Link
          href={`/agents/${agent.id}`}
          className="px-3 py-1.5 border border-gray-300 rounded text-sm hover:bg-gray-50 text-center"
        >
          Details
        </Link>
        <button
          onClick={handleDelete}
          disabled={loading}
          className="px-3 py-1.5 bg-red-600 text-white rounded text-sm hover:bg-red-700 disabled:opacity-50"
        >
          Delete
        </button>
      </div>
    </div>
  );
}
