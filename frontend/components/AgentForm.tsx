'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { createAgent, CreateAgentRequest } from '@/lib/api';

export default function AgentForm() {
  const router = useRouter();
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [formData, setFormData] = useState<CreateAgentRequest>({
    name: '',
    repo_url: '',
    branch: 'main',
    env_content: '',
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);
    setError('');

    try {
      await createAgent(formData);
      router.push('/');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create agent');
    } finally {
      setLoading(false);
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target;
    setFormData(prev => ({ ...prev, [name]: value }));
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-6 max-w-xl">
      {error && (
        <div className="bg-red-50 border border-red-200 text-red-700 px-4 py-3 rounded">
          {error}
        </div>
      )}

      <div>
        <label htmlFor="name" className="block text-sm font-medium text-gray-700">
          Agent Name
        </label>
        <input
          type="text"
          id="name"
          name="name"
          required
          value={formData.name}
          onChange={handleChange}
          className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          placeholder="my-agent"
        />
      </div>

      <div>
        <label htmlFor="repo_url" className="block text-sm font-medium text-gray-700">
          Repository URL
        </label>
        <input
          type="url"
          id="repo_url"
          name="repo_url"
          required
          value={formData.repo_url}
          onChange={handleChange}
          className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          placeholder="https://github.com/user/repo.git"
        />
      </div>

      <div>
        <label htmlFor="branch" className="block text-sm font-medium text-gray-700">
          Branch
        </label>
        <input
          type="text"
          id="branch"
          name="branch"
          value={formData.branch}
          onChange={handleChange}
          className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          placeholder="main"
        />
      </div>

      <div>
        <label htmlFor="env_content" className="block text-sm font-medium text-gray-700">
          Environment Variables
        </label>
        <textarea
          id="env_content"
          name="env_content"
          rows={6}
          value={formData.env_content}
          onChange={handleChange}
          className="mt-1 block w-full rounded-md border border-gray-300 px-3 py-2 shadow-sm focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 font-mono text-sm"
          placeholder="API_KEY=your-key&#10;DEBUG=true"
        />
        <p className="mt-1 text-sm text-gray-500">One variable per line in KEY=value format</p>
      </div>

      <div className="flex gap-4">
        <button
          type="submit"
          disabled={loading}
          className="flex-1 bg-blue-600 text-white px-4 py-2 rounded-md hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {loading ? 'Creating...' : 'Create Agent'}
        </button>
        <button
          type="button"
          onClick={() => router.push('/')}
          className="px-4 py-2 border border-gray-300 rounded-md hover:bg-gray-50 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2"
        >
          Cancel
        </button>
      </div>
    </form>
  );
}
