import test from 'node:test';
import assert from 'node:assert/strict';
import { mcpConfigDraftChanged, workspaceConfigDraftChanged } from './settingsDraft.js';

const savedWorkspaceConfig = {
  provider_id: 'provider-a',
  model: 'model-a',
  system_prompt: '保持简洁',
  context_mode: 'auto',
  max_context_messages: 12,
  temperature: 0.7,
  hide_thinking: false,
  embedding_base_url: 'http://embedding/v1',
  embedding_api_key: '',
  embedding_model: 'BAAI/bge-m3',
};

test('workspace config draft ignores derived display fields and numeric input types', () => {
  const current = {
    ...savedWorkspaceConfig,
    max_context_messages: '12',
    temperature: '0.7',
    base_url: 'derived-provider-url',
    models: ['model-a', 'model-b'],
    has_api_key: true,
  };
  assert.equal(workspaceConfigDraftChanged(current, savedWorkspaceConfig), false);
});

test('workspace config draft detects submitted field changes', () => {
  assert.equal(workspaceConfigDraftChanged({...savedWorkspaceConfig, model: 'model-b'}, savedWorkspaceConfig), true);
  assert.equal(workspaceConfigDraftChanged({...savedWorkspaceConfig, hide_thinking: true}, savedWorkspaceConfig), true);
  assert.equal(workspaceConfigDraftChanged({...savedWorkspaceConfig, embedding_api_key: 'new-key'}, savedWorkspaceConfig), true);
});

test('drafts are clean until a server baseline has loaded', () => {
  assert.equal(workspaceConfigDraftChanged(savedWorkspaceConfig, null), false);
  assert.equal(mcpConfigDraftChanged('{"servers":{}}', null), false);
});

test('MCP config draft normalizes line endings but preserves content changes', () => {
  assert.equal(mcpConfigDraftChanged('{\r\n  "servers": {}\r\n}\r\n', '{\n  "servers": {}\n}\n'), false);
  assert.equal(mcpConfigDraftChanged('{"servers":{"a":{}}}', '{"servers":{}}'), true);
});
