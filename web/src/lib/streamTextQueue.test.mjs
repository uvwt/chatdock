import assert from 'node:assert/strict';
import test from 'node:test';

import { createStreamTextQueue, streamRevealCount } from './streamTextQueue.js';

test('stream text queue reveals Unicode characters without breaking their order', () => {
  const queue = createStreamTextQueue();
  queue.enqueue({ reasoning: '思考', content: '你好😊' });

  assert.deepEqual(queue.take(3), [
    { kind: 'reasoning', text: '思考' },
    { kind: 'text', text: '你' },
  ]);
  assert.equal(queue.length, 2);
  assert.deepEqual(queue.drain(), [{ kind: 'text', text: '好😊' }]);
  assert.equal(queue.length, 0);
});

test('stream text queue merges adjacent segments and clears pending content', () => {
  const queue = createStreamTextQueue();
  queue.enqueue({ content: '第一' });
  queue.enqueue({ content: '第二' });
  assert.deepEqual(queue.take(3), [{ kind: 'text', text: '第一第' }]);
  queue.clear();
  assert.equal(queue.length, 0);
  assert.deepEqual(queue.drain(), []);
});

test('stream reveal count stays character-sized until backlog needs catching up', () => {
  assert.equal(streamRevealCount(0), 0);
  assert.equal(streamRevealCount(20), 1);
  assert.equal(streamRevealCount(49), 2);
  assert.equal(streamRevealCount(121), 4);
  assert.equal(streamRevealCount(241), 8);
});
