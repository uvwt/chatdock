import test from 'node:test';
import assert from 'node:assert/strict';
import { logoutAndReload } from './appUtils.js';

test('logoutAndReload clears the bearer token before reloading', () => {
  const calls = [];
  const storage = { removeItem: key => calls.push(['remove', key]) };
  const location = { reload: () => calls.push(['reload']) };

  logoutAndReload(storage, location);

  assert.deepEqual(calls, [
    ['remove', 'chatdock.authToken'],
    ['reload'],
  ]);
});
