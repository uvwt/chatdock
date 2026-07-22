export function uniqueModelNames(value) {
  const raw = Array.isArray(value) ? value.join('\n') : String(value || '');
  const seen = new Set();
  return raw.split(/[\n,，]+/).map(item => item.trim()).filter(Boolean).filter(item => {
    if (seen.has(item)) return false;
    seen.add(item);
    return true;
  });
}
export function providerKeyRows(provider = {}) {
  const keys = Array.isArray(provider?.api_keys) ? provider.api_keys : [];
  if (!keys.length) return [{id: 'main', name: '主 key', api_key: '', enabled: true, priority: 1}];
  return keys.map((key, index) => ({
    id: String(key.id || ('key-' + (index + 1))).trim(),
    name: String(key.name || key.id || ('Key ' + (index + 1))).trim(),
    api_key: key.api_key || key.api_key_masked || (key.has_api_key ? '********' : ''),
    enabled: key.enabled === false ? false : true,
    priority: Number(key.priority || index + 1) || index + 1,
    saved: !!(key.api_key || key.api_key_masked || key.has_api_key),
  }));
}

export function providerKeyInputsFromRows(rows, fallbackSecret = '') {
  const values = Array.isArray(rows) ? rows : [];
  const used = new Set();
  const clean = [];
  values.forEach((item, index) => {
    const secret = String(item?.api_key || '').trim();
    const saved = item?.saved === true || secret.includes('*');
    if (!secret && !saved && !fallbackSecret) return;
    let id = String(item?.id || '').trim();
    if (!id) id = index === 0 ? 'main' : 'key-' + (index + 1);
    while (used.has(id)) id = id + '-' + (index + 1);
    used.add(id);
    const name = String(item?.name || (index === 0 ? '主 key' : '备用 key ' + index)).trim();
    clean.push({ id, name, api_key: secret || fallbackSecret || '********', enabled: item?.enabled === false ? false : true, priority: clean.length + 1 });
  });
  return clean.length ? clean : null;
}

export function providerPayloadForModelAppend(provider, modelName) {
  const name = String(modelName || '').trim();
  const models = uniqueModelNames([...(provider?.models || []), name]);
  return {
    name: provider?.name || provider?.id || '',
    base_url: provider?.base_url || '',
    default_model: provider?.default_model || name,
    models,
    enabled: provider?.enabled !== false,
    key_strategy: provider?.key_strategy || 'auto',
    selected_key_id: provider?.selected_key_id || '',
    api_keys: providerKeyInputsFromRows(providerKeyRows(provider)) || undefined,
  };
}

export function providerPayloadFromFormValues(values = {}) {
  const apiKeys = providerKeyInputsFromRows(values.api_keys, '');
  const defaultModel = String(values.default_model || '').trim();
  const selectedKeyID = String(values.selected_key_id || '').trim();
  return {
    name: String(values.name || '').trim(),
    base_url: String(values.base_url || '').trim(),
    default_model: defaultModel,
    models: uniqueModelNames(values.models || defaultModel),
    enabled: values.enabled !== 'false',
    key_strategy: values.key_strategy || 'auto',
    selected_key_id: selectedKeyID || (apiKeys?.[0]?.id || ''),
    api_keys: apiKeys || undefined,
  };
}

export function providerChoiceID(provider) {
  return provider?.id || '';
}

export function providerLabel(provider) {
  return provider?.name || provider?.id || '供应商';
}

export function workspaceDefaultModelChoice(config = {}, provider = null) {
  const providerID = String(providerChoiceID(provider) || '').trim();
  const workspaceProviderID = String(config?.provider_id || '').trim();
  const workspaceModel = String(config?.model || '').trim();
  const providerModel = String(provider?.default_model || provider?.models?.[0] || '').trim();
  return {
    provider_id: providerID,
    model: !providerID || providerID === workspaceProviderID ? (workspaceModel || providerModel) : providerModel,
  };
}

export function compactModelName(name) {
  name = String(name || '').trim();
  return name.length > 22 ? name.slice(0, 21) + '…' : name;
}

export function sessionModelChoice(session) {
  return {
    provider_id: String(session?.provider_id || '').trim(),
    model: String(session?.model || '').trim(),
  };
}
