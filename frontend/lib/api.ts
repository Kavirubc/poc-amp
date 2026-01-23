const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8080';

export interface Agent {
  id: string;
  name: string;
  repo_url: string;
  branch: string;
  type: string;
  status: string;
  port: number;
  container_id?: string;
  image_id?: string;
  env_content?: string;
  error?: string;
  created_at: string;
  updated_at: string;
}

export interface CreateAgentRequest {
  name: string;
  repo_url: string;
  branch: string;
  env_content: string;
}

export interface AgentResponse {
  agent?: Agent;
  message?: string;
  error?: string;
}

export interface AgentsListResponse {
  agents: Agent[];
  total: number;
}

export async function listAgents(): Promise<AgentsListResponse> {
  const res = await fetch(`${API_BASE}/api/v1/agents`);
  if (!res.ok) throw new Error('Failed to fetch agents');
  return res.json();
}

export async function getAgent(id: string): Promise<AgentResponse> {
  const res = await fetch(`${API_BASE}/api/v1/agents/${id}`);
  if (!res.ok) throw new Error('Failed to fetch agent');
  return res.json();
}

export async function createAgent(data: CreateAgentRequest): Promise<AgentResponse> {
  const res = await fetch(`${API_BASE}/api/v1/agents`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) {
    const error = await res.json();
    throw new Error(error.error || 'Failed to create agent');
  }
  return res.json();
}

export async function deleteAgent(id: string): Promise<AgentResponse> {
  const res = await fetch(`${API_BASE}/api/v1/agents/${id}`, {
    method: 'DELETE',
  });
  if (!res.ok) throw new Error('Failed to delete agent');
  return res.json();
}

export async function startAgent(id: string): Promise<AgentResponse> {
  const res = await fetch(`${API_BASE}/api/v1/agents/${id}/start`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to start agent');
  return res.json();
}

export async function stopAgent(id: string): Promise<AgentResponse> {
  const res = await fetch(`${API_BASE}/api/v1/agents/${id}/stop`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to stop agent');
  return res.json();
}

export function getLogsUrl(id: string): string {
  return `${API_BASE}/api/v1/agents/${id}/logs`;
}
