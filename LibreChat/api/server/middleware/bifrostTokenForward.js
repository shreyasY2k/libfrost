const { logger } = require('@librechat/data-schemas');

/**
 * Middleware that forwards the user's OIDC access token and workspace ID
 * to Bifrost custom endpoint requests.
 *
 * When LibreChat proxies requests to custom endpoints (like Bifrost),
 * this middleware injects:
 * - Authorization: Bearer <OIDC access_token> (from federatedTokens)
 * - X-Workspace-ID: <workspace_id> (from client request header)
 *
 * This allows Bifrost's UserMgmt plugin to validate the JWT and resolve
 * workspace context for governance enforcement.
 *
 * Usage: Add to the middleware chain before custom endpoint proxy handlers.
 */
function bifrostTokenForward(req, res, next) {
  try {
    // Forward OIDC access token if available
    if (req.user?.federatedTokens?.access_token) {
      req.headers['authorization'] = `Bearer ${req.user.federatedTokens.access_token}`;
      logger.debug('[bifrostTokenForward] Forwarding OIDC access token to Bifrost');
    }

    // Forward workspace ID from client request
    const workspaceId = req.headers['x-workspace-id'];
    if (workspaceId) {
      logger.debug(`[bifrostTokenForward] Forwarding workspace ID: ${workspaceId}`);
    }
  } catch (error) {
    logger.warn('[bifrostTokenForward] Error forwarding tokens:', error.message);
  }

  next();
}

module.exports = bifrostTokenForward;
