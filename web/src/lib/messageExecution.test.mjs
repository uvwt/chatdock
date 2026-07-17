import test from 'node:test';
import assert from 'node:assert/strict';
import { assistantMessageBlocks, executionBlockSummary } from './messageExecution.js';

test('assistantMessageBlocks preserves reasoning, tools, and answer order', () => {
  const firstTool = {kind: 'tool', phase: 'done', text: '读取文件'};
  const secondTool = {kind: 'tool', phase: 'done', text: '检查状态'};
  const result = assistantMessageBlocks({
    parts: [
      {kind: 'reasoning', text: '先分析'},
      {kind: 'tool', event: firstTool},
      {kind: 'tool', event: secondTool},
      {kind: 'reasoning', text: '继续判断'},
      {kind: 'text', text: '最终回答'},
    ],
  });

  assert.deepEqual(result.blocks.map(block => block.kind), ['reasoning', 'tools', 'reasoning', 'text']);
  assert.deepEqual(result.blocks[1].events, [firstTool, secondTool]);
  assert.deepEqual(result.textParts, ['最终回答']);
});

test('assistantMessageBlocks only groups adjacent tool calls', () => {
  const result = assistantMessageBlocks({
    parts: [
      {kind: 'tool', event: {phase: 'done', text: '工具一'}},
      {kind: 'reasoning', text: '中间思考'},
      {kind: 'tool', event: {phase: 'done', text: '工具二'}},
    ],
  });

  assert.deepEqual(result.blocks.map(block => block.kind), ['tools', 'reasoning', 'tools']);
  assert.equal(result.blocks[0].events.length, 1);
  assert.equal(result.blocks[2].events.length, 1);
});

test('assistantMessageBlocks keeps pending confirmations visible', () => {
  const confirmation = {kind: 'confirm', status: 'pending', text: '等待确认'};
  const result = assistantMessageBlocks({
    content: '回答',
    events: [{kind: 'tool', phase: 'done', text: '完成调用'}, confirmation],
  });

  assert.deepEqual(result.blocks.map(block => block.kind), ['tools', 'confirmation', 'text']);
  assert.equal(result.blocks[1].event, confirmation);
});

test('assistantMessageBlocks hides reasoning without changing remaining order', () => {
  const result = assistantMessageBlocks({
    parts: [
      {kind: 'reasoning', text: '内部思考'},
      {kind: 'tool', event: {phase: 'done', text: '读取'}},
      {kind: 'text', text: '回答'},
    ],
  }, {hideThinking: true});

  assert.deepEqual(result.blocks.map(block => block.kind), ['tools', 'text']);
  assert.deepEqual(result.textParts, ['回答']);
});

test('assistantMessageBlocks falls back for incomplete historical parts', () => {
  const result = assistantMessageBlocks({
    content: '历史正文',
    reasoning: '历史思考',
    events: [{kind: 'tool', phase: 'done', text: '历史工具'}],
    parts: [{kind: 'attachment', text: ''}],
  });

  assert.deepEqual(result.blocks.map(block => block.kind), ['reasoning', 'tools', 'text']);
  assert.deepEqual(result.textParts, ['历史正文']);
});

test('executionBlockSummary reports reasoning, running, and failed blocks', () => {
  assert.deepEqual(
    executionBlockSummary({kind: 'reasoning', text: '检查配置和当前运行状态'}, {streaming: true}),
    {label: '正在思考', meta: '检查配置和当前运行状态', tone: 'running'},
  );
  assert.deepEqual(
    executionBlockSummary({kind: 'tools', events: [{phase: 'running', text: '搜索资料'}]}, {streaming: true}),
    {label: '正在调用 搜索资料', meta: '执行中', tone: 'running'},
  );
  assert.deepEqual(
    executionBlockSummary({kind: 'tools', events: [{phase: 'done', text: '读取'}, {phase: 'error', text: '写入'}]}),
    {label: '工具调用存在失败', meta: '读取、写入 · 1 项失败', tone: 'error'},
  );
});
