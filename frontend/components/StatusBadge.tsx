'use client';

interface StatusBadgeProps {
  status: string;
}

const statusConfig: Record<string, { color: string; label: string }> = {
  pending: { color: 'bg-yellow-100 text-yellow-800', label: 'Pending' },
  cloning: { color: 'bg-blue-100 text-blue-800', label: 'Cloning' },
  building: { color: 'bg-purple-100 text-purple-800', label: 'Building' },
  running: { color: 'bg-green-100 text-green-800', label: 'Running' },
  stopped: { color: 'bg-gray-100 text-gray-800', label: 'Stopped' },
  failed: { color: 'bg-red-100 text-red-800', label: 'Failed' },
};

export default function StatusBadge({ status }: StatusBadgeProps) {
  const config = statusConfig[status] || { color: 'bg-gray-100 text-gray-800', label: status };

  return (
    <span className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${config.color}`}>
      {config.label}
    </span>
  );
}
