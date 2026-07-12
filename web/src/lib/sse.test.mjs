import assert from 'node:assert/strict';
import test from 'node:test';

import { readSSE, SSEStreamClosedError } from './sse.js';

function streamResponse(...chunks) {
  const encoder = new TextEncoder();
  return new Response(new ReadableStream({
    start(controller) {
      for (const chunk of chunks) controller.enqueue(encoder.encode(chunk));
      controller.close();
    },
  }), {headers: {'Content-Type': 'text/event-stream'}});
}

test('readSSE parses event ids, ignores heartbeats, and requires a terminal event', async () => {
  const events = [];
  const result = await readSSE(streamResponse(
    ': heartbeat\r\n\r\nid: 1\r\nevent: delta\r\ndata: {"content":"第一段"}\r\n\r\n',
    'id: 2\nevent: delta\ndata: {"content":"第二段"}\n\nevent: done\ndata: {"status":"success"}\n\n',
  ), (event, data, id) => events.push({event, data, id}));

  assert.equal(result.lastEventID, 2);
  assert.deepEqual(events, [
    {event: 'delta', data: {content: '第一段'}, id: 1},
    {event: 'delta', data: {content: '第二段'}, id: 2},
    {event: 'done', data: {status: 'success'}, id: 0},
  ]);
});

test('readSSE reports premature EOF with the last persisted event id', async () => {
  const events = [];
  await assert.rejects(
    readSSE(streamResponse('id: 7\nevent: delta\ndata: {"content":"尚未完成"}\n\n'), (event, data, id) => events.push({event, data, id})),
    error => {
      assert.ok(error instanceof SSEStreamClosedError);
      assert.equal(error.code, 'SSE_STREAM_CLOSED');
      assert.equal(error.lastEventID, 7);
      return true;
    },
  );
  assert.deepEqual(events, [{event: 'delta', data: {content: '尚未完成'}, id: 7}]);
});

test('failed message_end is a terminal stream event', async () => {
  const result = await readSSE(
    streamResponse('event: message_end\ndata: {"status":"failed"}\n\n'),
    () => {},
  );
  assert.equal(result.lastEventID, 0);
});
