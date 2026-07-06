import test from 'node:test';
import assert from 'node:assert/strict';
import { actualToolCall, buildToolEventDetail, hasDisplayValue } from './toolEventDetails.js';

test('hasDisplayValue treats empty values as hidden', () => {
  assert.equal(hasDisplayValue(''), false);
  assert.equal(hasDisplayValue([]), false);
  assert.equal(hasDisplayValue({a: 1}), true);
});

test('actualToolCall unwraps execute proxy arguments', () => {
  const actual = actualToolCall({tool: 'chatdock_tool_execute', arguments: {name: 'memory_read', arguments: {id: '1'}}, result: {result: {ok: true}}});
  assert.equal(actual.actualTool, 'memory_read');
  assert.deepEqual(actual.actualArguments, {id: '1'});
});

test('actualToolCall marks parse errors clearly', () => {
  const actual = actualToolCall({tool: 'chatdock_tool_execute', arguments: {_parse_error: 'name is required'}});
  assert.equal(actual.mode, 'execute_parse_error');
  assert.equal(actual.actualTool, '工具参数解析失败');
});

test('buildToolEventDetail creates search summary with collapsed response', () => {
  const detail = buildToolEventDetail({details: {event: 'tool_result', tool: 'chatdock_tools_search', arguments: {query: 'mail'}, result: {tools: ['gmail.read', 'gmail.search']}, data: {ok: true, tool: 'chatdock_tools_search', arguments: {query: 'mail'}, result: {tools: ['gmail.read', 'gmail.search']}}}});
  assert.equal(detail.heading, '找到 2 个候选工具');
  assert.equal(detail.primary.name, '2 个候选工具');
  assert.ok(detail.sections.some(section => section.title === '候选工具'));
});
