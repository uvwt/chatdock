import assert from 'node:assert/strict';
import test from 'node:test';

import { isTextEntryTarget, normalizeViewportMetrics, shouldKeepMessagesAtBottom } from './viewportLayout.js';

test('normalizeViewportMetrics uses the visual viewport when the keyboard shrinks and pans it', () => {
  assert.deepEqual(normalizeViewportMetrics({ height: 412.4, offsetTop: 47.6 }, 844), {
    height: 412,
    offsetTop: 48,
  });
});

test('normalizeViewportMetrics falls back to the window height', () => {
  assert.deepEqual(normalizeViewportMetrics(null, 780.8), {
    height: 781,
    offsetTop: 0,
  });
});

test('isTextEntryTarget recognizes editable controls', () => {
  assert.equal(isTextEntryTarget({ tagName: 'TEXTAREA' }), true);
  assert.equal(isTextEntryTarget({ tagName: 'DIV', isContentEditable: true }), true);
  assert.equal(isTextEntryTarget({ tagName: 'BUTTON' }), false);
});

test('shouldKeepMessagesAtBottom only follows a conversation near its bottom edge', () => {
  assert.equal(shouldKeepMessagesAtBottom({ scrollHeight: 1000, scrollTop: 490, clientHeight: 400 }), true);
  assert.equal(shouldKeepMessagesAtBottom({ scrollHeight: 1000, scrollTop: 300, clientHeight: 400 }), false);
  assert.equal(shouldKeepMessagesAtBottom(null), false);
});
