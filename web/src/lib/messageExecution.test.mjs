import test from 'node:test';
import assert from 'node:assert/strict';
import { executionSummary, splitAssistantMessage } from './messageExecution.js';

test('splitAssistantMessage keeps answer first and groups execution details', () => {
  const message = {
    parts: [
      {kind: 'reasoning', text: '先分析'},
      {kind: 'tool', event: {kind: 'tool', phase: 'done', text: '读取文件'}},
      {kind: 'reasoning', text: '继续判断'},
      {kind: 'text', text: '最终回答'},
    ],
  };

  const result = splitAssistantMessage(message);
  assert.deepEqual(result.textParts, ['最终回答']);
  assert.deepEqual(result.reasoningParts, ['先分析', '继续判断']);
  assert.equal(result.executionEvents.length, 1);
  assert.equal(result.confirmations.length, 0);
});

test('splitAssistantMessage keeps pending confirmations outside execution history', () => {
  const confirmation = {kind: 'confirm', status: 'pending', text: '等待确认'};
  const result = splitAssistantMessage({events: [confirmation, {kind: 'tool', phase: 'done', text: '完成调用'}]});

  assert.deepEqual(result.confirmations, [confirmation]);
  assert.equal(result.executionEvents.length, 1);
});

test('splitAssistantMessage respects hidden thinking', () => {
  const result = splitAssistantMessage({reasoning: '内部思考', content: '回答'}, {hideThinking: true});
  assert.deepEqual(result.textParts, ['回答']);
  assert.deepEqual(result.reasoningParts, []);
});

test('splitAssistantMessage falls back when historical parts are incomplete', () => {
  const result = splitAssistantMessage({
    content: '历史正文',
    reasoning: '历史思考',
    events: [{kind: 'tool', phase: 'done', text: '历史工具'}],
    parts: [{kind: 'attachment', text: ''}],
  });

  assert.deepEqual(result.textParts, ['历史正文']);
  assert.deepEqual(result.reasoningParts, ['历史思考']);
  assert.equal(result.executionEvents.length, 1);
});

test('executionSummary reports active and failed states', () => {
  assert.deepEqual(
    executionSummary({events: [{phase: 'running', text: '搜索资料'}], streaming: true}),
    {label: '正在调用 搜索资料', meta: '执行中', tone: 'running'},
  );
  assert.deepEqual(
    executionSummary({events: [{phase: 'done', text: '读取'}, {phase: 'error', text: '写入'}], reasoningParts: ['分析']}),
    {label: '执行过程存在失败', meta: '2 项执行记录 · 1 段思考 · 1 项失败', tone: 'error'},
  );
});
