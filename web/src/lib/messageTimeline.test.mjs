import test from 'node:test';
import assert from 'node:assert/strict';
import { formatMessageTimeDivider, shouldShowMessageTimeDivider } from './messageTimeline.js';

function localTime(year, month, day, hour, minute) {
  return new Date(year, month - 1, day, hour, minute, 0, 0);
}

test('message time divider appears for the first message, long pauses, and date boundaries', () => {
  const first = {created_at: localTime(2026, 7, 25, 10, 0)};

  assert.equal(shouldShowMessageTimeDivider(null, first), true);
  assert.equal(shouldShowMessageTimeDivider(first, {created_at: localTime(2026, 7, 25, 10, 29)}), false);
  assert.equal(shouldShowMessageTimeDivider(first, {created_at: localTime(2026, 7, 25, 10, 30)}), true);
  assert.equal(shouldShowMessageTimeDivider(
    {created_at: localTime(2026, 7, 25, 23, 55)},
    {created_at: localTime(2026, 7, 26, 0, 5)},
  ), true);
  assert.equal(shouldShowMessageTimeDivider(null, {created_at: 'invalid'}), false);
  assert.equal(shouldShowMessageTimeDivider(first, {created_at: 'invalid'}), false);
});

test('message time divider uses compact Chinese date labels', () => {
  const now = localTime(2026, 7, 25, 18, 0);

  assert.equal(formatMessageTimeDivider(localTime(2026, 7, 25, 9, 5), now), '09:05');
  assert.equal(formatMessageTimeDivider(localTime(2026, 7, 24, 23, 8), now), '昨天 23:08');
  assert.equal(formatMessageTimeDivider(localTime(2026, 6, 3, 7, 6), now), '6月3日 07:06');
  assert.equal(formatMessageTimeDivider(localTime(2025, 12, 31, 22, 15), now), '2025年12月31日 22:15');
});
