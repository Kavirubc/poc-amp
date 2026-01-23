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

// Compensation Mapping types
export interface CompensationMapping {
  id: string;
  agent_id: string;
  tool_name: string;
  tool_schema?: Record<string, unknown>;
  tool_description?: string;
  compensator_name?: string;
  parameter_mapping?: Record<string, string>;
  status: 'pending' | 'approved' | 'rejected' | 'no_compensation';
  suggested_by: 'heuristic' | 'llm' | 'manual';
  confidence: number;
  reasoning?: string;
  reviewed_by?: string;
  reviewed_at?: string;
  created_at: string;
  updated_at: string;
}

export interface CompensationMappingsResponse {
  mappings: CompensationMapping[];
  total: number;
}

export interface ToolSchema {
  name: string;
  description: string;
  inputSchema?: Record<string, unknown>;
}

export interface RollbackStep {
  transaction_id: string;
  tool_name: string;
  original_input: Record<string, unknown>;
  original_output: Record<string, unknown>;
  action: 'compensate' | 'skip';
  compensator_name?: string;
  compensation_params?: Record<string, unknown>;
  reason?: string;
}

export interface RollbackPlan {
  agent_id: string;
  session_id: string;
  steps: RollbackStep[];
  total: number;
}

// Agent APIs
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

// Compensation Mapping APIs
export async function registerTools(agentId: string, tools: ToolSchema[]): Promise<CompensationMappingsResponse> {
  const res = await fetch(`${API_BASE}/api/v1/agents/${agentId}/tools`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ tools }),
  });
  if (!res.ok) throw new Error('Failed to register tools');
  return res.json();
}

export async function listCompensationMappings(agentId: string): Promise<CompensationMappingsResponse> {
  const res = await fetch(`${API_BASE}/api/v1/agents/${agentId}/compensation-mappings`);
  if (!res.ok) throw new Error('Failed to fetch compensation mappings');
  return res.json();
}

export async function approveMapping(mappingId: string, data?: { compensator_name?: string; parameter_mapping?: Record<string, string> }): Promise<{ mapping: CompensationMapping; message: string }> {
  const res = await fetch(`${API_BASE}/api/v1/compensation-mappings/${mappingId}/approve`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data || {}),
  });
  if (!res.ok) throw new Error('Failed to approve mapping');
  return res.json();
}

export async function rejectMapping(mappingId: string, noCompensation: boolean = false): Promise<{ mapping: CompensationMapping; message: string }> {
  const res = await fetch(`${API_BASE}/api/v1/compensation-mappings/${mappingId}/reject`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ no_compensation: noCompensation }),
  });
  if (!res.ok) throw new Error('Failed to reject mapping');
  return res.json();
}

export async function updateMapping(mappingId: string, data: { compensator_name?: string; parameter_mapping?: Record<string, string> }): Promise<{ mapping: CompensationMapping; message: string }> {
  const res = await fetch(`${API_BASE}/api/v1/compensation-mappings/${mappingId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  });
  if (!res.ok) throw new Error('Failed to update mapping');
  return res.json();
}

export async function getRollbackPlan(agentId: string, sessionId: string): Promise<RollbackPlan> {
  const res = await fetch(`${API_BASE}/api/v1/agents/${agentId}/sessions/${sessionId}/rollback-plan`);
  if (!res.ok) throw new Error('Failed to get rollback plan');
  return res.json();
}

export async function executeRollback(agentId: string, sessionId: string): Promise<{ result: { total_transactions: number; compensated: number; failed: number; skipped: number }; plan: RollbackStep[] }> {
  const res = await fetch(`${API_BASE}/api/v1/agents/${agentId}/sessions/${sessionId}/rollback`, {
    method: 'POST',
  });
  if (!res.ok) throw new Error('Failed to execute rollback');
  return res.json();
}
