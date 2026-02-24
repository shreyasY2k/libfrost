import { memo, useState, useRef, useEffect } from 'react';
import { useWorkspace, type Workspace } from '~/hooks/useWorkspace';

/**
 * WorkspaceSelector renders a dropdown in the sidebar for selecting
 * the active workspace. The selection is persisted to localStorage
 * and injected into API requests via the X-Workspace-ID header.
 */
function WorkspaceSelector() {
  const { workspaces, selectedWorkspace, selectWorkspace, loading } = useWorkspace();
  const [isOpen, setIsOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Close dropdown when clicking outside
  useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    }
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  // Don't render if no workspaces available
  if (!loading && workspaces.length === 0) {
    return null;
  }

  return (
    <div ref={dropdownRef} className="relative mx-2 mb-2">
      <button
        type="button"
        onClick={() => setIsOpen(!isOpen)}
        className="flex w-full items-center justify-between rounded-lg border border-border-light bg-surface-secondary px-3 py-2 text-sm text-text-primary transition-colors hover:bg-surface-tertiary"
        aria-haspopup="listbox"
        aria-expanded={isOpen}
        aria-label="Select workspace"
      >
        <div className="flex items-center gap-2 truncate">
          <svg
            className="h-4 w-4 flex-shrink-0 text-text-secondary"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
            strokeWidth={2}
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              d="M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16m14 0h2m-2 0h-5m-9 0H3m2 0h5M9 7h1m-1 4h1m4-4h1m-1 4h1m-5 10v-5a1 1 0 011-1h2a1 1 0 011 1v5m-4 0h4"
            />
          </svg>
          <span className="truncate">
            {loading ? 'Loading...' : selectedWorkspace?.name ?? 'Select workspace'}
          </span>
        </div>
        <svg
          className={`h-4 w-4 flex-shrink-0 text-text-secondary transition-transform ${isOpen ? 'rotate-180' : ''}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {isOpen && (
        <div
          className="absolute left-0 right-0 z-50 mt-1 max-h-60 overflow-auto rounded-lg border border-border-light bg-surface-primary shadow-lg"
          role="listbox"
          aria-label="Workspaces"
        >
          {workspaces.map((workspace: Workspace) => (
            <button
              key={workspace.id}
              type="button"
              role="option"
              aria-selected={workspace.id === selectedWorkspace?.id}
              className={`flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-surface-tertiary ${
                workspace.id === selectedWorkspace?.id
                  ? 'bg-surface-secondary font-medium text-text-primary'
                  : 'text-text-secondary'
              }`}
              onClick={() => {
                selectWorkspace(workspace.id);
                setIsOpen(false);
              }}
            >
              <span className="truncate">{workspace.name}</span>
              {workspace.description && (
                <span className="ml-auto truncate text-xs text-text-tertiary">
                  {workspace.description}
                </span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

export default memo(WorkspaceSelector);
