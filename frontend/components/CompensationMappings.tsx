'use client';

import { useState, useEffect, useCallback } from 'react';
import {
  CompensationMapping,
  listCompensationMappings,
  approveMapping,
  rejectMapping,
  registerTools,
  ToolSchema,
} from '@/lib/api';

interface CompensationMappingsProps {
  agentId: string;
}

const statusColors: Record<string, string> = {
  pending: 'bg-yellow-100 text-yellow-800',
  approved: 'bg-green-100 text-green-800',
  rejected: 'bg-red-100 text-red-800',
  no_compensation: 'bg-gray-100 text-gray-800',
};

const sourceLabels: Record<string, string> = {
  heuristic: 'Pattern Match',
  llm: 'AI Suggested',
  manual: 'Manual',
};

export default function CompensationMappings({ agentId }: CompensationMappingsProps) {
  const [mappings, setMappings] = useState<CompensationMapping[]>([]);
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [showRegisterModal, setShowRegisterModal] = useState(false);
  const [toolsJson, setToolsJson] = useState('');

  const fetchMappings = useCallback(async () => {
    try {
      const data = await listCompensationMappings(agentId);
      setMappings(data.mappings || []);
    } catch (err) {
      console.error('Failed to fetch mappings:', err);
    } finally {
      setLoading(false);
    }
  }, [agentId]);

  useEffect(() => {
    fetchMappings();
  }, [fetchMappings]);

  const handleApprove = async (mappingId: string) => {
    setActionLoading(mappingId);
    try {
      await approveMapping(mappingId);
      fetchMappings();
    } catch (err) {
      console.error('Failed to approve:', err);
    } finally {
      setActionLoading(null);
    }
  };

  const handleReject = async (mappingId: string, noCompensation: boolean) => {
    setActionLoading(mappingId);
    try {
      await rejectMapping(mappingId, noCompensation);
      fetchMappings();
    } catch (err) {
      console.error('Failed to reject:', err);
    } finally {
      setActionLoading(null);
    }
  };

  const handleRegisterTools = async () => {
    try {
      const tools: ToolSchema[] = JSON.parse(toolsJson);
      await registerTools(agentId, tools);
      setShowRegisterModal(false);
      setToolsJson('');
      fetchMappings();
    } catch (err) {
      console.error('Failed to register tools:', err);
      alert('Invalid JSON or failed to register tools');
    }
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center py-8">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-600"></div>
      </div>
    );
  }

  const pendingMappings = mappings.filter(m => m.status === 'pending');
  const approvedMappings = mappings.filter(m => m.status === 'approved');
  const otherMappings = mappings.filter(m => m.status !== 'pending' && m.status !== 'approved');

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-lg font-semibold">Compensation Mappings</h2>
        <button
          onClick={() => setShowRegisterModal(true)}
          className="px-4 py-2 bg-blue-600 text-white rounded-md text-sm hover:bg-blue-700"
        >
          Register Tools
        </button>
      </div>

      {mappings.length === 0 ? (
        <div className="text-center py-8 bg-gray-50 rounded-lg border border-gray-200">
          <p className="text-gray-600">No tools registered yet</p>
          <p className="text-sm text-gray-500 mt-1">
            Register tools to get compensation suggestions
          </p>
        </div>
      ) : (
        <>
          {/* Pending Approvals */}
          {pendingMappings.length > 0 && (
            <div className="space-y-4">
              <h3 className="text-md font-medium text-yellow-700 flex items-center gap-2">
                <span className="w-2 h-2 bg-yellow-500 rounded-full"></span>
                Pending Review ({pendingMappings.length})
              </h3>
              {pendingMappings.map(mapping => (
                <MappingCard
                  key={mapping.id}
                  mapping={mapping}
                  onApprove={() => handleApprove(mapping.id)}
                  onReject={(noComp) => handleReject(mapping.id, noComp)}
                  loading={actionLoading === mapping.id}
                />
              ))}
            </div>
          )}

          {/* Approved Mappings */}
          {approvedMappings.length > 0 && (
            <div className="space-y-4">
              <h3 className="text-md font-medium text-green-700 flex items-center gap-2">
                <span className="w-2 h-2 bg-green-500 rounded-full"></span>
                Approved ({approvedMappings.length})
              </h3>
              {approvedMappings.map(mapping => (
                <MappingCard
                  key={mapping.id}
                  mapping={mapping}
                  onApprove={() => {}}
                  onReject={() => {}}
                  loading={false}
                  readonly
                />
              ))}
            </div>
          )}

          {/* Other Mappings */}
          {otherMappings.length > 0 && (
            <div className="space-y-4">
              <h3 className="text-md font-medium text-gray-600">
                Other ({otherMappings.length})
              </h3>
              {otherMappings.map(mapping => (
                <MappingCard
                  key={mapping.id}
                  mapping={mapping}
                  onApprove={() => {}}
                  onReject={() => {}}
                  loading={false}
                  readonly
                />
              ))}
            </div>
          )}
        </>
      )}

      {/* Register Tools Modal */}
      {showRegisterModal && (
        <div className="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-50">
          <div className="bg-white rounded-lg p-6 w-full max-w-2xl max-h-[80vh] overflow-auto">
            <h3 className="text-lg font-semibold mb-4">Register Tools</h3>
            <p className="text-sm text-gray-600 mb-4">
              Paste the JSON array of tool schemas to analyze for compensation mappings.
            </p>
            <textarea
              value={toolsJson}
              onChange={(e) => setToolsJson(e.target.value)}
              className="w-full h-64 p-3 border border-gray-300 rounded-md font-mono text-sm"
              placeholder={`[
  {
    "name": "book_flight",
    "description": "Books a flight reservation",
    "inputSchema": {
      "type": "object",
      "properties": {
        "flight_id": { "type": "string" },
        "passenger_id": { "type": "string" }
      }
    }
  }
]`}
            />
            <div className="flex justify-end gap-3 mt-4">
              <button
                onClick={() => setShowRegisterModal(false)}
                className="px-4 py-2 border border-gray-300 rounded-md hover:bg-gray-50"
              >
                Cancel
              </button>
              <button
                onClick={handleRegisterTools}
                className="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700"
              >
                Register & Analyze
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

interface MappingCardProps {
  mapping: CompensationMapping;
  onApprove: () => void;
  onReject: (noCompensation: boolean) => void;
  loading: boolean;
  readonly?: boolean;
}

function MappingCard({ mapping, onApprove, onReject, loading, readonly }: MappingCardProps) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="bg-white border border-gray-200 rounded-lg p-4 shadow-sm">
      <div className="flex items-start justify-between">
        <div className="flex-1">
          <div className="flex items-center gap-3">
            <code className="text-lg font-semibold text-gray-900">{mapping.tool_name}</code>
            <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${statusColors[mapping.status]}`}>
              {mapping.status.replace('_', ' ')}
            </span>
            <span className="text-xs text-gray-500">
              {sourceLabels[mapping.suggested_by] || mapping.suggested_by}
            </span>
          </div>
          {mapping.tool_description && (
            <p className="text-sm text-gray-600 mt-1">{mapping.tool_description}</p>
          )}
        </div>
        {mapping.confidence > 0 && (
          <div className="text-right">
            <div className="text-2xl font-bold text-blue-600">
              {Math.round(mapping.confidence * 100)}%
            </div>
            <div className="text-xs text-gray-500">confidence</div>
          </div>
        )}
      </div>

      {mapping.compensator_name && (
        <div className="mt-4 p-3 bg-gray-50 rounded-md">
          <div className="text-sm">
            <span className="text-gray-600">Suggested Compensator: </span>
            <code className="font-semibold text-green-700">{mapping.compensator_name}</code>
          </div>
          {mapping.parameter_mapping && Object.keys(mapping.parameter_mapping).length > 0 && (
            <div className="mt-2">
              <div className="text-xs text-gray-500 mb-1">Parameter Mapping:</div>
              <div className="space-y-1">
                {Object.entries(mapping.parameter_mapping).map(([param, source]) => (
                  <div key={param} className="flex items-center gap-2 text-sm">
                    <code className="bg-white px-2 py-0.5 rounded border">{param}</code>
                    <span className="text-gray-400">&larr;</span>
                    <code className="text-blue-600">{source}</code>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      )}

      {mapping.reasoning && (
        <div className="mt-3">
          <button
            onClick={() => setExpanded(!expanded)}
            className="text-sm text-blue-600 hover:underline"
          >
            {expanded ? 'Hide reasoning' : 'Show reasoning'}
          </button>
          {expanded && (
            <p className="mt-2 text-sm text-gray-600 bg-blue-50 p-3 rounded-md">
              {mapping.reasoning}
            </p>
          )}
        </div>
      )}

      {!readonly && mapping.status === 'pending' && (
        <div className="mt-4 flex gap-2 border-t border-gray-100 pt-4">
          <button
            onClick={onApprove}
            disabled={loading}
            className="flex-1 bg-green-600 text-white px-4 py-2 rounded-md text-sm hover:bg-green-700 disabled:opacity-50"
          >
            Approve
          </button>
          <button
            onClick={() => onReject(true)}
            disabled={loading}
            className="px-4 py-2 border border-gray-300 rounded-md text-sm hover:bg-gray-50 disabled:opacity-50"
          >
            No Compensation Needed
          </button>
          <button
            onClick={() => onReject(false)}
            disabled={loading}
            className="px-4 py-2 bg-red-100 text-red-700 rounded-md text-sm hover:bg-red-200 disabled:opacity-50"
          >
            Reject
          </button>
        </div>
      )}

      {mapping.reviewed_at && (
        <div className="mt-3 text-xs text-gray-500">
          Reviewed by {mapping.reviewed_by || 'admin'} on {new Date(mapping.reviewed_at).toLocaleString()}
        </div>
      )}
    </div>
  );
}
