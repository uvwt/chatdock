import test from 'node:test';
import assert from 'node:assert/strict';
import { appendInlineReasoningPart, appendInlineTextPart, appendToolStartEvent, mergeToolResultEvent } from './toolEvents.js';

test('appendInlineTextPart merges adjacent text parts', () => {
  const msg = appendInlineTextPart(appendInlineTextPart({}, '你'), '好');
  assert.equal(msg.answer, '你好');
  assert.deepEqual(msg.parts, [{kind: 'text', text: '你好'}]);
});

test('appendInlineReasoningPart merges adjacent reasoning parts', () => {
  const msg = appendInlineReasoningPart(appendInlineReasoningPart({}, '思'), '考');
  assert.equal(msg.reasoning, '思考');
  assert.deepEqual(msg.parts, [{kind: 'reasoning', text: '思考'}]);
});

test('mergeToolResultEvent updates matching running event', () => {
  const started = appendToolStartEvent({}, 'tool_start', {tool: 'chatdock_tool_execute', arguments: {name: 'search', arguments: {q: 'x'}}});
  const merged = mergeToolResultEvent(started, 'tool_result', {ok: true, tool: 'chatdock_tool_execute', result: {tool: 'search', result: 'ok'}});
  assert.equal(merged.events.length, 1);
  assert.equal(merged.events[0].phase, 'done');
  assert.equal(merged.parts[0].event.phase, 'done');
});
