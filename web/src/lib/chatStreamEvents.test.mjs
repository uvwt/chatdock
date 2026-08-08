import assert from 'node:assert/strict';
import test from 'node:test';

import { chatStreamAssistantAfterEvent, chatStreamStatsAfterEvent, messagesForRunningJobReplay, projectsChatStreamAssistant } from './chatStreamEvents.js';

const idleStats = () => ({ state: 'idle', started_at: 1, chars: 0, events: 0, tools: 0, error: '' });

test('messagesForRunningJobReplay replaces the persisted assistant checkpoint before replaying events', () => {
  const messages = [
    { role: 'user', content: '看下火山额度', created_at: '2026-07-25T16:02:31Z' },
    { role: 'assistant', content: '先检查一下 Skill 的', reasoning: '旧思考', events: [{ text: '读取文件' }], created_at: '2026-07-25T16:02:38Z' },
  ];
  const next = messagesForRunningJobReplay(messages, { id: 'job-1', started_at: '2026-07-25T16:02:31Z' });

  assert.equal(next.length, 2);
  assert.equal(next.at(-1).role, 'assistant-stream');
  assert.equal(next.at(-1).job_id, 'job-1');
  assert.equal(next.at(-1).answer, '');
  assert.equal(next.at(-1).reasoning, '');
  assert.equal(next.at(-1).events.length, 1);
  assert.equal(next.at(-1).created_at, '2026-07-25T16:02:38Z');
  assert.equal(messages.at(-1).role, 'assistant');
});

test('messagesForRunningJobReplay appends a stream after the user message when no checkpoint exists', () => {
  const next = messagesForRunningJobReplay(
    [{ role: 'user', content: '继续', created_at: '2026-07-25T16:02:31Z' }],
    { id: 'job-2', started_at: '2026-07-25T16:02:32Z' },
  );

  assert.equal(next.length, 2);
  assert.equal(next.at(-1).role, 'assistant-stream');
  assert.equal(next.at(-1).created_at, '2026-07-25T16:02:32Z');
});

test('messagesForRunningJobReplay resets an existing stream before replaying from event zero', () => {
  const next = messagesForRunningJobReplay([
    { role: 'user', content: '继续' },
    { role: 'assistant-stream', job_id: 'job-3', answer: '重复内容', events: [{ text: '旧事件' }], created_at: '2026-07-25T16:02:33Z' },
  ], { id: 'job-3', started_at: '2026-07-25T16:02:32Z' });

  assert.equal(next.length, 2);
  assert.equal(next.at(-1).answer, '');
  assert.equal(next.at(-1).events.length, 1);
  assert.equal(next.at(-1).events[0].text, '已恢复后台生成');
});

test('chatStreamStatsAfterEvent tracks streamed characters and pause state', () => {
  const next = chatStreamStatsAfterEvent(idleStats(), 'delta', { content: 'abc', reasoning_content: '思考' }, true);
  assert.equal(next.state, 'paused');
  assert.equal(next.chars, 5);
});

test('chatStreamStatsAfterEvent counts tool and lifecycle events', () => {
  const tool = chatStreamStatsAfterEvent(idleStats(), 'tool_call_start', {});
  assert.equal(tool.events, 1);
  assert.equal(tool.tools, 1);
  const failed = chatStreamStatsAfterEvent(tool, 'message_end', { status: 'failed' });
  assert.equal(failed.state, 'error');
});

test('tool setup success stays out of the visible assistant timeline', () => {
  assert.equal(projectsChatStreamAssistant('tool_setup_ready', { tool_count: 4 }), false);
  const message = { role: 'assistant-stream', answer: '', events: [] };
  assert.equal(chatStreamAssistantAfterEvent(message, 'tool_setup_ready', { tool_count: 4 }), message);
});

test('model retry is projected as a visible execution event', () => {
  assert.equal(projectsChatStreamAssistant('model_retry', {}), true);
  const stats = chatStreamStatsAfterEvent(idleStats(), 'model_retry', {});
  assert.equal(stats.events, 1);

  const message = chatStreamAssistantAfterEvent({ role: 'assistant-stream', events: [] }, 'model_retry', {
    provider_id: 'primary',
    model: 'primary-model',
    attempt: 1,
    max_retries: 2,
    delay_ms: 500,
    reason: 'unexpected EOF',
  });
  assert.equal(message.events.length, 1);
  assert.equal(message.events[0].text, '重新连接模型');
  assert.equal(message.events[0].meta, 'primary · primary-model · 1/2');
  assert.equal(message.events[0].details.data.reason, 'unexpected EOF');
});

test('model fallback is projected as a visible execution event', () => {
  assert.equal(projectsChatStreamAssistant('model_fallback', {}), true);
  const stats = chatStreamStatsAfterEvent(idleStats(), 'model_fallback', {});
  assert.equal(stats.events, 1);

  const message = chatStreamAssistantAfterEvent({ role: 'assistant-stream', events: [] }, 'model_fallback', {
    from_provider_id: 'primary',
    from_model: 'primary-model',
    to_provider_id: 'backup',
    to_model: 'backup-model',
    reason: '上游不可用',
  });
  assert.equal(message.events.length, 1);
  assert.equal(message.events[0].text, '切换备用模型');
  assert.equal(message.events[0].meta, 'backup · backup-model');
  assert.equal(message.events[0].details.data.reason, '上游不可用');
});

test('chatStreamAssistantAfterEvent projects tool and confirmation events', () => {
  const started = chatStreamAssistantAfterEvent({ role: 'assistant-stream', events: [] }, 'tool_call_start', {
    tool: 'DockMini__exec_command',
    arguments: { cmd: 'pwd' },
  });
  assert.equal(started.events.length, 1);
  assert.match(started.events[0].text, /DockMini__exec_command/);

  const waiting = chatStreamAssistantAfterEvent(started, 'tool_confirmation_required', {
    id: 'confirm-1',
    tool: 'DockMini__exec_command',
    arguments: { cmd: 'pwd' },
  });
  const resolved = chatStreamAssistantAfterEvent(waiting, 'tool_confirmation_resolved', {
    id: 'confirm-1',
    tool: 'DockMini__exec_command',
    approved: true,
  });
  assert.equal(resolved.events.at(-1).status, 'resolved');
  assert.match(resolved.events.at(-1).text, /已允许/);
});

test('run protocol events only project meaningful non-tool records', () => {
  assert.equal(projectsChatStreamAssistant('run_event', { kind: 'tool_call' }), false);
  assert.equal(projectsChatStreamAssistant('run_event', { kind: 'summary' }), true);
  const next = chatStreamAssistantAfterEvent({ events: [] }, 'run_event', {
    kind: 'summary',
    status: 'success',
    summary: '运行结束',
    duration_ms: 18,
  });
  assert.equal(next.events.length, 1);
  assert.match(next.events[0].meta, /成功/);
});

test('error event updates both stats and preserves raw details on the assistant message', () => {
  const data = { message: '模型不可用', raw: 'dial tcp: connection refused', code: 'UPSTREAM_UNAVAILABLE', request_id: 'req_stream' };
  const stats = chatStreamStatsAfterEvent(idleStats(), 'error', data);
  assert.equal(stats.state, 'error');
  assert.match(stats.error, /模型不可用/);
  const message = chatStreamAssistantAfterEvent({ answer: '', events: [] }, 'error', data);
  assert.equal(message.events[0].phase, 'error');
  assert.match(message.events[0].text, /模型不可用/);
  assert.equal(message.error.raw, 'dial tcp: connection refused');
  assert.equal(message.error.request_id, 'req_stream');
});
