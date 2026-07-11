import assert from 'node:assert/strict';
import test from 'node:test';

import { chatStreamAssistantAfterEvent, chatStreamStatsAfterEvent, projectsChatStreamAssistant } from './chatStreamEvents.js';

const idleStats = () => ({ state: 'idle', started_at: 1, chars: 0, events: 0, tools: 0, error: '' });

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

test('error event updates both stats and assistant message', () => {
  const data = { message: '模型不可用' };
  const stats = chatStreamStatsAfterEvent(idleStats(), 'error', data);
  assert.equal(stats.state, 'error');
  assert.match(stats.error, /模型不可用/);
  const message = chatStreamAssistantAfterEvent({ answer: '', events: [] }, 'error', data);
  assert.equal(message.events[0].phase, 'error');
  assert.match(message.events[0].text, /模型不可用/);
});
