const WORKSPACE_STORAGE_KEY = 'enterprise-workspace-id';

/**
 * Returns headers to inject into API requests to Bifrost.
 * Includes the X-Workspace-ID header from the currently selected workspace.
 *
 * Usage:
 *   const headers = getBifrostHeaders();
 *   fetch(url, { headers: { ...headers } });
 */
export function getBifrostHeaders(): Record<string, string> {
  const headers: Record<string, string> = {};

  try {
    const workspaceId = localStorage.getItem(WORKSPACE_STORAGE_KEY);
    if (workspaceId) {
      headers['X-Workspace-ID'] = workspaceId;
    }
  } catch {
    // localStorage unavailable
  }

  return headers;
}

/**
 * Injects Bifrost headers into an existing headers object.
 * Mutates the provided headers map in place.
 */
export function injectBifrostHeaders(headers: Record<string, string>): void {
  const bifrostHeaders = getBifrostHeaders();
  Object.assign(headers, bifrostHeaders);
}
