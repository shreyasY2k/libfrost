import { useState, useEffect, useCallback, useMemo } from 'react';
import { useAuthContext } from '~/hooks';

export interface Workspace {
  id: string;
  name: string;
  description?: string;
  org_id: string;
  team_id?: string;
  allowed_models?: string[];
  is_active: boolean;
}

const WORKSPACE_STORAGE_KEY = 'enterprise-workspace-id';
const BIFROST_BASE_URL = '/api/enterprise';

/**
 * Custom hook for managing workspace state and API calls.
 * Fetches workspaces from the Bifrost API, persists selection in localStorage,
 * and provides the selected workspace context for API calls.
 */
export function useWorkspace() {
  const { token, isAuthenticated } = useAuthContext();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(() => {
    try {
      return localStorage.getItem(WORKSPACE_STORAGE_KEY);
    } catch {
      return null;
    }
  });
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchWorkspaces = useCallback(async () => {
    if (!isAuthenticated || !token) {
      return;
    }

    setLoading(true);
    setError(null);

    try {
      const response = await fetch(`${BIFROST_BASE_URL}/workspaces`, {
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json',
        },
      });

      if (!response.ok) {
        throw new Error(`Failed to fetch workspaces: ${response.statusText}`);
      }

      const data: Workspace[] = await response.json();
      setWorkspaces(data);

      // Auto-select first workspace if none selected
      if (!selectedId && data.length > 0) {
        setSelectedId(data[0].id);
        localStorage.setItem(WORKSPACE_STORAGE_KEY, data[0].id);
      }

      // Validate current selection still exists
      if (selectedId && !data.find((ws) => ws.id === selectedId)) {
        if (data.length > 0) {
          setSelectedId(data[0].id);
          localStorage.setItem(WORKSPACE_STORAGE_KEY, data[0].id);
        } else {
          setSelectedId(null);
          localStorage.removeItem(WORKSPACE_STORAGE_KEY);
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch workspaces');
    } finally {
      setLoading(false);
    }
  }, [isAuthenticated, token, selectedId]);

  useEffect(() => {
    fetchWorkspaces();
  }, [fetchWorkspaces]);

  const selectWorkspace = useCallback((workspaceId: string) => {
    setSelectedId(workspaceId);
    try {
      localStorage.setItem(WORKSPACE_STORAGE_KEY, workspaceId);
    } catch {
      // localStorage may be unavailable
    }
  }, []);

  const selectedWorkspace = useMemo(
    () => workspaces.find((ws) => ws.id === selectedId) ?? null,
    [workspaces, selectedId],
  );

  return {
    workspaces,
    selectedWorkspace,
    selectedId,
    selectWorkspace,
    loading,
    error,
    refetch: fetchWorkspaces,
  };
}
