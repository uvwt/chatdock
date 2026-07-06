import test from 'node:test';
import assert from 'node:assert/strict';
import { compactModelName, providerKeyInputsFromRows, providerKeyRows, uniqueModelNames } from './modelProviderForm.js';

test('uniqueModelNames trims separators and deduplicates', () => {
  assert.deepEqual(uniqueModelNames('a\na，b,b'), ['a', 'b']);
});

test('providerKeyRows masks saved keys', () => {
  const rows = providerKeyRows({api_keys: [{id: 'main', has_api_key: true, priority: 3}]});
  assert.equal(rows[0].api_key, '********');
  assert.equal(rows[0].saved, true);
  assert.equal(rows[0].priority, 3);
});

test('providerKeyInputsFromRows keeps saved masks', () => {
  assert.deepEqual(providerKeyInputsFromRows([{id: 'main', name: '主 key', api_key: '********', saved: true}]), [{id: 'main', name: '主 key', api_key: '********', enabled: true, priority: 1}]);
});

test('compactModelName shortens long names', () => {
  assert.equal(compactModelName('1234567890123456789012345'), '123456789012345678901…');
});
