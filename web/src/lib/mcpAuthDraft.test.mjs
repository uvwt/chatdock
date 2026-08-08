import test from 'node:test';
import assert from 'node:assert/strict';
import {
  mcpAuthDraft,
  mcpAuthPayload,
  mcpTokenReferenceField,
  normalizeBearerTokenDraft,
} from './mcpAuthDraft.js';

test('saved MCP token becomes an empty replacement field', () => {
  const draft = mcpAuthDraft({
    type: 'bearer',
    [mcpTokenReferenceField]: 'DockMini',
  });
  assert.equal(draft.token, '');
  assert.equal(draft.saved_token_ref, 'DockMini');
  assert.equal(draft.auth_type, 'bearer');
});

test('blank token keeps the saved reference while a new token replaces it', () => {
  const preserved = mcpAuthPayload({
    auth_type: 'bearer',
    token: '',
    saved_token_ref: 'DockMini',
  });
  assert.deepEqual(preserved, {
    type: 'bearer',
    [mcpTokenReferenceField]: 'DockMini',
  });

  const replaced = mcpAuthPayload({
    auth_type: 'bearer',
    token: '  Bearer new-token  ',
    saved_token_ref: 'DockMini',
  });
  assert.deepEqual(replaced, {type: 'bearer', token: 'new-token', [mcpTokenReferenceField]: 'DockMini'});
});

test('clearing an unsaved replacement returns to keeping the saved token', () => {
  const withReplacement = mcpAuthPayload({
    auth_type: 'bearer',
    token: 'new-token',
    saved_token_ref: 'DockMini',
  });
  const draft = mcpAuthDraft(withReplacement);
  assert.deepEqual(mcpAuthPayload({...draft, token: ''}), {
    type: 'bearer',
    [mcpTokenReferenceField]: 'DockMini',
  });
});

test('explicitly clearing the saved reference removes inline authentication', () => {
  assert.equal(mcpAuthPayload({
    auth_type: 'none',
    token: '',
    saved_token_ref: '',
  }), null);
  assert.deepEqual(mcpAuthPayload({
    auth_type: 'bearer',
    token: '',
    saved_token_ref: '',
  }), {type: 'bearer'});
});

test('disabling authentication does not silently discard an existing token', () => {
  assert.deepEqual(mcpAuthPayload({
    auth_type: 'none',
    saved_token_ref: 'DockMini',
  }), {
    type: 'none',
    [mcpTokenReferenceField]: 'DockMini',
  });
});

test('Bearer prefix normalization is case insensitive', () => {
  assert.equal(normalizeBearerTokenDraft('  bEaReR abc  '), 'abc');
  assert.equal(normalizeBearerTokenDraft('abc'), 'abc');
});
