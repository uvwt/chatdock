import assert from 'node:assert/strict';
import test from 'node:test';

import { streamChat } from './chatApi.js';

function streamResponse(body) {
  return new Response(body, {status: 200, headers: {'Content-Type': 'text/event-stream'}});
}

test('streamChat reconnects after premature EOF and resumes after the last event id', async () => {
  const originalFetch = globalThis.fetch;
  const requests = [];
  globalThis.fetch = async (url, options = {}) => {
    requests.push({url: String(url), options});
    if (requests.length === 1) {
      assert.equal(url, '/api/chat/stream');
      assert.equal(options.method, 'POST');
      return streamResponse([
        'event: job_started\ndata: {"id":"job-1"}\n\n',
        'id: 1\nevent: delta\ndata: {"content":"第一段"}\n\n',
      ].join(''));
    }
    assert.equal(url, '/api/chat/jobs/job-1/events?after=1');
    return streamResponse([
      'id: 2\nevent: delta\ndata: {"content":"第二段"}\n\n',
      'event: message_end\ndata: {"status":"success"}\n\n',
      'event: done\ndata: {"status":"success","session":{"id":"session-1"}}\n\n',
    ].join(''));
  };

  try {
    const events = [];
    await streamChat({
      authHeaders: extra => ({Authorization: 'Bearer test', ...extra}),
      signal: new AbortController().signal,
      sessionID: 'session-1',
      message: '测试续传',
      attachmentIDs: [],
      providerID: 'default',
      model: 'test-model',
      onEvent: (event, data) => events.push({event, data}),
    });

    assert.equal(requests.length, 2);
    assert.deepEqual(events.map(item => item.event), ['job_started', 'delta', 'delta', 'message_end', 'done']);
    assert.equal(events.filter(item => item.event === 'delta').map(item => item.data.content).join(''), '第一段第二段');
    assert.equal(events.at(-1).data.session.id, 'session-1');
  } finally {
    globalThis.fetch = originalFetch;
  }
});
