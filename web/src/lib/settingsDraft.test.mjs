import test from 'node:test';
import assert from 'node:assert/strict';
import { mcpConfigDraftChanged, globalConfigDraftChanged, unsavedSettingsPrompt, validateMCPConfigRaw } from './settingsDraft.js';

const savedGlobalConfig = {
  provider_id: 'provider-a',
  model: 'model-a',
  fallback_provider_id: '',
  fallback_model: '',
  system_prompt: '保持简洁',
  context_mode: 'auto',
  max_context_messages: 12,
  temperature: 0.7,
  hide_thinking: false,
  embedding_base_url: 'http://embedding/v1',
  embedding_api_key: '',
  embedding_model: 'BAAI/bge-m3',
};

test('global config draft ignores derived display fields and numeric input types', () => {
  const current = {
    ...savedGlobalConfig,
    max_context_messages: '12',
    temperature: '0.7',
    base_url: 'derived-provider-url',
    models: ['model-a', 'model-b'],
    has_api_key: true,
  };
  assert.equal(globalConfigDraftChanged(current, savedGlobalConfig), false);
});

test('global config draft detects submitted field changes', () => {
  assert.equal(globalConfigDraftChanged({...savedGlobalConfig, model: 'model-b'}, savedGlobalConfig), true);
  assert.equal(globalConfigDraftChanged({...savedGlobalConfig, fallback_provider_id: 'provider-b', fallback_model: 'model-c'}, savedGlobalConfig), true);
  assert.equal(globalConfigDraftChanged({...savedGlobalConfig, hide_thinking: true}, savedGlobalConfig), true);
  assert.equal(globalConfigDraftChanged({...savedGlobalConfig, embedding_api_key: 'new-key'}, savedGlobalConfig), true);
});

test('drafts are clean until a server baseline has loaded', () => {
  assert.equal(globalConfigDraftChanged(savedGlobalConfig, null), false);
  assert.equal(mcpConfigDraftChanged('{"servers":{}}', null), false);
});

test('MCP config draft normalizes line endings but preserves content changes', () => {
  assert.equal(mcpConfigDraftChanged('{\r\n  "servers": {}\r\n}\r\n', '{\n  "servers": {}\n}\n'), false);
  assert.equal(mcpConfigDraftChanged('{"servers":{"a":{}}}', '{"servers":{}}'), true);
});

test('MCP raw config validation rejects blank content without replacing it', () => {
  assert.deepEqual(validateMCPConfigRaw(''), {ok: false, error: 'MCP 配置不能为空。'});
  assert.deepEqual(validateMCPConfigRaw(' \n\t '), {ok: false, error: 'MCP 配置不能为空。'});
});

test('MCP raw config validation rejects invalid JSON and non-object roots', () => {
  assert.equal(validateMCPConfigRaw('{').ok, false);
  assert.match(validateMCPConfigRaw('{').error, /不是合法 JSON/);
  assert.deepEqual(validateMCPConfigRaw('[]'), {ok: false, error: 'MCP 配置必须是 JSON 对象。'});
});

test('MCP raw config validation accepts object JSON and preserves original content', () => {
  const raw = '{\n  "servers": {}\n}\n';
  const result = validateMCPConfigRaw(raw);
  assert.equal(result.ok, true);
  assert.equal(result.content, raw);
  assert.deepEqual(result.config, {servers: {}});
});

test('unsaved settings prompt names only the dirty scopes', () => {
  assert.equal(unsavedSettingsPrompt('leave', false, false), '');
  assert.equal(unsavedSettingsPrompt('leave', true, false), '离开配置中心会丢弃尚未保存的模型配置，确定继续吗？');
  assert.equal(unsavedSettingsPrompt('refresh', true, true), '刷新配置中心会丢弃尚未保存的模型配置和工具配置，确定继续吗？');
});
