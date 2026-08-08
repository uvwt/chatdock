function finiteNumber(value, fallback) {
  const number = Number(value);
  return Number.isFinite(number) ? number : fallback;
}

// 只比较真正会提交到全局配置接口的字段，避免 providers/models 等派生展示数据误报未保存。
export function globalConfigDraftSignature(config = {}) {
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

export function globalConfigDraftChanged(current, saved) {
  if (!saved) return false;
  return globalConfigDraftSignature(current) !== globalConfigDraftSignature(saved);
}

function normalizeLineEndings(value) {
  return String(value || '').replace(/\r\n?/g, '\n');
}

export function mcpConfigDraftChanged(current, saved) {
  if (saved == null) return false;
  return normalizeLineEndings(current) !== normalizeLineEndings(saved);
}

export function validateMCPConfigRaw(content) {
  const raw = String(content ?? '');
  if (!raw.trim()) return {ok: false, error: 'MCP 配置不能为空。'};
  try {
    const config = JSON.parse(raw);
    if (!config || typeof config !== 'object' || Array.isArray(config)) {
      return {ok: false, error: 'MCP 配置必须是 JSON 对象。'};
    }
    return {ok: true, content: raw, config, error: ''};
  } catch (error) {
    return {ok: false, error: 'MCP 配置不是合法 JSON：' + error.message};
  }
}

export function unsavedSettingsPrompt(action, configDirty, mcpConfigDirty) {
  const scopes = [];
  if (configDirty) scopes.push('模型配置');
  if (mcpConfigDirty) scopes.push('工具配置');
  if (!scopes.length) return '';
  const prefix = action === 'refresh' ? '刷新配置中心' : '离开配置中心';
  return `${prefix}会丢弃尚未保存的${scopes.join('和')}，确定继续吗？`;
}
