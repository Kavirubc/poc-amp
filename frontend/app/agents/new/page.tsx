import AgentForm from '@/components/AgentForm';

export default function NewAgentPage() {
  return (
    <div>
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Create New Agent</h1>
        <p className="text-gray-600">Deploy a new AI agent from a Git repository</p>
      </div>
      <div className="bg-white rounded-lg border border-gray-200 p-6">
        <AgentForm />
      </div>
    </div>
  );
}
