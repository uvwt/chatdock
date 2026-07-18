import assert from 'node:assert/strict';
import test from 'node:test';

import { isTextEntryTarget, normalizeViewportMetrics } from './viewportLayout.js';

test('normalizeViewportMetrics uses the visual viewport when the keyboard shrinks it', () => {
  assert.deepEqual(normalizeViewportMetrics({ height: 412.4 }, 844), {
    height: 412,
  });
});

test('normalizeViewportMetrics falls back to the window height', () => {
  assert.deepEqual(normalizeViewportMetrics(null, 780.8), {
    height: 781,
  });
});

test('isTextEntryTarget recognizes editable controls', () => {
  assert.equal(isTextEntryTarget({ tagName: 'TEXTAREA' }), true);
  assert.equal(isTextEntryTarget({ tagName: 'DIV', isContentEditable: true }), true);
  assert.equal(isTextEntryTarget({ tagName: 'BUTTON' }), false);
});
