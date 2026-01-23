'use client';

import { useEffect, useState, useCallback } from 'react';
import { Agent, listAgents } from '@/lib/api';
import AgentCard from '@/components/AgentCard';

export default function Dashboard() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  const fetchAgents = useCallback(async () => {
    try {
      const data = await listAgents();
      setAgents(data.agents || []);
      setError('');
    } catch (err) {
      setError('Failed to fetch agents');
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchAgents();
    const interval = setInterval(fetchAgents, 5000);
    return () => clearInterval(interval);
  }, [fetchAgents]);

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="animate-spin rounded-full h-12 w-12 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
        {error}
      </div>
    );
  }

  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Agents</h1>
        <p className="text-gray-600">Manage your deployed AI agents</p>
      </div>

      {agents.length === 0 ? (
        <div className="text-center py-12 bg-white rounded-lg border border-gray-200">
          <h3 className="text-lg font-medium text-gray-900 mb-2">No agents yet</h3>
          <p className="text-gray-600 mb-4">Get started by creating your first agent</p>
          <a
            href="/agents/new"
            className="inline-flex items-center px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
          >
            Create Agent
          </a>
        </div>
      ) : (
        <div className="grid gap-6 md:grid-cols-2 lg:grid-cols-3">
          {agents.map(agent => (
            <AgentCard key={agent.id} agent={agent} onUpdate={fetchAgents} />
          ))}
        </div>
      )}
    </div>
  );
}
