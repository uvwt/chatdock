import test from 'node:test';
import assert from 'node:assert/strict';
import { attachmentLooksLikeImage, contextPreviewText, finalAssistantMessageFromSession, readableChatError, scheduledTaskRunsText, streamErrorMessage, streamStatusText } from './chatPresentation.js';

test('streamStatusText formats visible streaming state', () => {
  assert.equal(streamStatusText({state: 'streaming', chars: 12, tools: 1, events: 2}, 3), '流式输出中 · 3s · 12 字 · 1 个工具 · 2 个事件');
});

test('readableChatError extracts provider message and request id', () => {
  const err = {message: 'model api failed: {"error":{"message":"bad model"}}', request_id: 'req_1'};
  assert.equal(readableChatError(err), 'bad model');
  assert.match(readableChatError({message: 'boom', request_id: 'req_2'}), /请求 ID：req_2/);
});

test('readableChatError explains image unsupported errors', () => {
  assert.match(readableChatError('model only support text input', true), /不能读取图片/);
});

test('streamErrorMessage appends request id', () => {
  assert.equal(streamErrorMessage({message: '中断', request_id: 'abc'}), '中断\n请求 ID：abc');
});

test('finalAssistantMessageFromSession returns latest assistant message', () => {
  const msg = {role: 'assistant', content: 'last'};
  assert.equal(finalAssistantMessageFromSession({messages: [{role: 'assistant', content: 'old'}, {role: 'user'}, msg]}), msg);
});

test('attachmentLooksLikeImage recognizes mime aliases', () => {
  assert.equal(attachmentLooksLikeImage({mime_type: 'image/png'}), true);
  assert.equal(attachmentLooksLikeImage({type: 'text/plain'}), false);
});

test('scheduledTaskRunsText and contextPreviewText produce copyable diagnostics', () => {
  assert.match(scheduledTaskRunsText({title: '日报', context_mode: 'session'}, []), /连续会话/);
  assert.match(contextPreviewText({workspace: 'default', items: [{role: 'user', chars: 2, estimated_tokens: 1, content_preview: 'hi'}]}), /工作空间：default/);
});
