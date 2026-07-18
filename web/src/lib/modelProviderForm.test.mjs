import test from 'node:test';
import assert from 'node:assert/strict';
import { compactModelName, providerKeyInputsFromRows, providerKeyRows, providerPayloadForModelAppend, providerPayloadFromFormValues, uniqueModelNames } from './modelProviderForm.js';

test('uniqueModelNames trims separators and deduplicates', () => {
  assert.deepEqual(uniqueModelNames('a\na，b,b'), ['a', 'b']);
});

test('providerKeyRows masks saved keys', () => {
  const rows = providerKeyRows({api_keys: [{id: 'main', has_api_key: true, priority: 3}]});
  assert.equal(rows[0].api_key, '********');
  assert.equal(rows[0].saved, true);
  assert.equal(rows[0].priority, 3);
});

test('providerKeyRows creates a blank key row for a new provider', () => {
  assert.deepEqual(providerKeyRows(null), [{id: 'main', name: '主 key', api_key: '', enabled: true, priority: 1}]);
});

test('providerKeyInputsFromRows keeps saved masks', () => {
  assert.deepEqual(providerKeyInputsFromRows([{id: 'main', name: '主 key', api_key: '********', saved: true}]), [{id: 'main', name: '主 key', api_key: '********', enabled: true, priority: 1}]);
});

test('providerPayloadForModelAppend keeps the current snake_case contract', () => {
  const payload = providerPayloadForModelAppend({
    id: 'provider-a',
    name: 'Provider A',
    base_url: 'https://example.test/v1',
    default_model: 'model-a',
    models: ['model-a'],
    selected_key_id: 'main',
    api_keys: [{id: 'main', name: '主 key', has_api_key: true, enabled: true, priority: 1}],
  }, 'model-b');
  assert.deepEqual(payload.models, ['model-a', 'model-b']);
  assert.equal(payload.api_keys[0].api_key, '********');
  assert.equal('api_key' in payload, false);
  assert.equal('apiKeys' in payload, false);
});

test('providerPayloadFromFormValues trims and normalizes the save payload', () => {
  const payload = providerPayloadFromFormValues({
    name: ' Demo ',
    base_url: ' https://example.test/v1 ',
    default_model: ' model-a ',
    models: 'model-a\nmodel-b\nmodel-a',
    enabled: 'false',
    api_keys: [{id: 'main', name: '主 key', api_key: 'secret'}],
  });
  assert.equal(payload.name, 'Demo');
  assert.equal(payload.base_url, 'https://example.test/v1');
  assert.equal(payload.default_model, 'model-a');
  assert.deepEqual(payload.models, ['model-a', 'model-b']);
  assert.equal(payload.enabled, false);
  assert.equal(payload.selected_key_id, 'main');
});

test('compactModelName shortens long names', () => {
  assert.equal(compactModelName('1234567890123456789012345'), '123456789012345678901…');
});
