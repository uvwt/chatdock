export const mcpTokenReferenceField = '_chatdock_token_ref';

export function normalizeBearerTokenDraft(value) {
  return String(value || '').trim().replace(/^Bearer\s+/i, '');
}

export function mcpAuthDraft(auth = {}) {
  const savedTokenRef = String(auth[mcpTokenReferenceField] || '').trim();
  const token = String(auth.token || '');
  return {
    auth_type: auth.type || (token || auth.token_env || savedTokenRef ? 'bearer' : 'none'),
    token,
    token_env: auth.token_env || '',
    saved_token_ref: savedTokenRef,
  };
}

export function mcpAuthPayload(draft = {}) {
  const authType = String(draft.auth_type || '').trim() || 'none';
  const token = normalizeBearerTokenDraft(draft.token);
  const tokenEnv = String(draft.token_env || '').trim();
  const savedTokenRef = String(draft.saved_token_ref || '').trim();
  if (authType === 'none' && !savedTokenRef) return null;

  const auth = {type: authType};
  if (token) auth.token = token;
  // 草稿阶段保留旧值引用，用户输入后再清空时仍能回到“留空保持”的状态。
  // 服务端收到新 Token 时会优先替换并移除这个引用。
  if (savedTokenRef) auth[mcpTokenReferenceField] = savedTokenRef;
  if (authType !== 'none' && tokenEnv) auth.token_env = tokenEnv;
  return auth;
}
