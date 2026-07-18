import assert from 'node:assert/strict';
import test from 'node:test';

import {
  IOS_KEYBOARD_ACCESSORY_INSET,
  isIOSKeyboardAccessoryDevice,
  isTextEntryTarget,
  keyboardAccessoryInset,
  normalizeViewportMetrics,
  shouldKeepMessagesAtBottom,
} from './viewportLayout.js';

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

test('isIOSKeyboardAccessoryDevice recognizes iPhone and desktop-mode iPad', () => {
  assert.equal(isIOSKeyboardAccessoryDevice({ userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)' }), true);
  assert.equal(isIOSKeyboardAccessoryDevice({ platform: 'MacIntel', maxTouchPoints: 5 }), true);
  assert.equal(isIOSKeyboardAccessoryDevice({ userAgent: 'Mozilla/5.0 (Linux; Android 15)', platform: 'Linux armv8l', maxTouchPoints: 5 }), false);
});

test('keyboardAccessoryInset only reserves the iOS input assistant for the composer', () => {
  const textarea = { id: 'input' };
  const composerShell = { contains: target => target === textarea };
  const iphone = { userAgent: 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)' };

  assert.equal(keyboardAccessoryInset(iphone, textarea, composerShell), IOS_KEYBOARD_ACCESSORY_INSET);
  assert.equal(keyboardAccessoryInset(iphone, { id: 'search' }, composerShell), 0);
  assert.equal(keyboardAccessoryInset({ userAgent: 'Mozilla/5.0 (Linux; Android 15)' }, textarea, composerShell), 0);
});

test('shouldKeepMessagesAtBottom only follows a conversation near its bottom edge', () => {
  assert.equal(shouldKeepMessagesAtBottom({ scrollHeight: 1000, scrollTop: 490, clientHeight: 400 }), true);
  assert.equal(shouldKeepMessagesAtBottom({ scrollHeight: 1000, scrollTop: 300, clientHeight: 400 }), false);
  assert.equal(shouldKeepMessagesAtBottom(null), false);
});
