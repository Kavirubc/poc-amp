import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'AMP - Agent Management Platform',
  description: 'Manage and deploy AI agents',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="en">
      <body className="bg-gray-50 min-h-screen">
        <nav className="bg-white border-b border-gray-200">
          <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
            <div className="flex justify-between h-16 items-center">
              <a href="/" className="text-xl font-bold text-gray-900">
                AMP
              </a>
              <a
                href="/agents/new"
                className="bg-blue-600 text-white px-4 py-2 rounded-md text-sm hover:bg-blue-700"
              >
                New Agent
              </a>
            </div>
          </div>
        </nav>
        <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
          {children}
        </main>
      </body>
    </html>
  );
}
