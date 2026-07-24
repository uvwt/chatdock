import test from 'node:test';
import assert from 'node:assert/strict';
import { attachmentLooksLikeImage, chatErrorDetails, contextPreviewText, finalAssistantMessageFromSession, readableChatError, streamErrorMessage, streamStatusText } from './chatPresentation.js';

test('streamStatusText formats visible streaming state', () => {
  assert.equal(streamStatusText({state: 'streaming', chars: 12, tools: 1, events: 2}, 3), '流式输出中 · 3s · 12 字 · 1 个工具 · 2 个事件');
});

test('readableChatError extracts the provider message without duplicating metadata', () => {
  const err = {message: 'model api failed: {"error":{"message":"bad model"}}', request_id: 'req_1'};
  assert.equal(readableChatError(err), 'bad model');
  assert.equal(readableChatError({message: 'boom', request_id: 'req_2'}), 'boom');
});

test('readableChatError explains image unsupported errors', () => {
  assert.match(readableChatError('model only support text input', true), /不能读取图片/);
});

test('streamErrorMessage keeps request metadata outside the message', () => {
  assert.equal(streamErrorMessage({message: '中断', request_id: 'abc'}), '中断');
});

test('chatErrorDetails preserves raw error metadata for the details panel', () => {
  assert.deepEqual(chatErrorDetails({
    message: '友好提示',
    raw: 'dial tcp: connection refused',
    code: 'UPSTREAM_UNAVAILABLE',
    request_id: 'req_3',
    retryable: true,
  }), {
    message: '友好提示',
    raw: 'dial tcp: connection refused',
    code: 'UPSTREAM_UNAVAILABLE',
    request_id: 'req_3',
    retryable: true,
  });
});

test('finalAssistantMessageFromSession returns latest assistant message', () => {
  const msg = {role: 'assistant', content: 'last'};
  assert.equal(finalAssistantMessageFromSession({messages: [{role: 'assistant', content: 'old'}, {role: 'user'}, msg]}), msg);
});

test('attachmentLooksLikeImage recognizes mime aliases', () => {
  assert.equal(attachmentLooksLikeImage({mime_type: 'image/png'}), true);
  assert.equal(attachmentLooksLikeImage({type: 'text/plain'}), false);
});

test('contextPreviewText produces copyable diagnostics', () => {
  assert.match(contextPreviewText({project_name: 'Alpha', items: [{role: 'user', chars: 2, estimated_tokens: 1, content_preview: 'hi'}]}), /项目：Alpha/);
  assert.match(contextPreviewText({items: []}), /普通会话/);
});
