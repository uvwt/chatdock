function finiteNumber(value, fallback) {
  const number = Number(value);
  return Number.isFinite(number) ? number : fallback;
}

// 只比较真正会提交到工作空间配置接口的字段，避免 providers/models 等派生展示数据误报未保存。
export function workspaceConfigDraftSignature(config = {}) {
  return JSON.stringify({
    provider_id: String(config.provider_id || ''),
    model: String(config.model || ''),
    fallback_provider_id: String(config.fallback_provider_id || ''),
    fallback_model: String(config.fallback_model || ''),
    system_prompt: String(config.system_prompt || ''),
    context_mode: String(config.context_mode || 'auto'),
    max_context_messages: finiteNumber(config.max_context_messages, 12),
    temperature: finiteNumber(config.temperature, 0.7),
    hide_thinking: !!config.hide_thinking,
    embedding_base_url: String(config.embedding_base_url || ''),
    embedding_api_key: String(config.embedding_api_key || ''),
    embedding_model: String(config.embedding_model || 'BAAI/bge-m3'),
  });
}

export function workspaceConfigDraftChanged(current, saved) {
  if (!saved) return false;
  return workspaceConfigDraftSignature(current) !== workspaceConfigDraftSignature(saved);
}

function normalizeLineEndings(value) {
  return String(value || '').replace(/\r\n?/g, '\n');
}

export function mcpConfigDraftChanged(current, saved) {
  if (saved == null) return false;
  return normalizeLineEndings(current) !== normalizeLineEndings(saved);
}
