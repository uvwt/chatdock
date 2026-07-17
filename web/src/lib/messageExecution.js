function fallbackAssistantText(message, streaming) {
  if (streaming) return String(message?.answer || '');
  return String(message?.content || message?.answer || '');
}

function pendingConfirmation(event) {
  return event?.kind === 'confirm' && event.status !== 'resolved';
}

function appendToolBlock(blocks, event) {
  const last = blocks[blocks.length - 1];
  if (last?.kind === 'tools') {
    last.events.push(event);
    return;
  }
  blocks.push({kind: 'tools', events: [event]});
}

function insertBeforeFirstText(blocks, block) {
  const index = blocks.findIndex(item => item.kind === 'text');
  if (index < 0) blocks.push(block);
  else blocks.splice(index, 0, block);
}

export function toolEventDisplayName(event) {
  const details = event?.details || {};
  const data = details.data || {};
  return String(
    details.arguments?.name ||
    data.arguments?.name ||
    data.result?.tool ||
    details.tool ||
    data.tool ||
    event?.text ||
    event?.meta ||
    ''
  ).trim();
}

export function toolEventMetaText(event) {
  const meta = String(event?.meta || '').replace(/^关键词：/, '').trim();
  if (meta) return meta;
  const details = event?.details || {};
  const query = details.arguments?.query || details.data?.arguments?.query || details.data?.result?.query;
  return query ? String(query).trim() : '';
}

export function assistantMessageBlocks(message, { streaming = false, hideThinking = false } = {}) {
  const parts = Array.isArray(message?.parts) ? message.parts : [];
  const blocks = [];
  let hasTextPart = false;
  let hasReasoningPart = false;
  let hasToolPart = false;

  // parts 是流式协议记录的真实时间线；这里只合并相邻工具，不改变思考、工具和正文的先后顺序。
  for (const part of parts) {
    if (part?.kind === 'text' && part.text) {
      hasTextPart = true;
      blocks.push({kind: 'text', text: String(part.text)});
      continue;
    }
    if (part?.kind === 'reasoning' && part.text) {
      hasReasoningPart = true;
      if (!hideThinking) blocks.push({kind: 'reasoning', text: String(part.text)});
      continue;
    }
    if (part?.kind === 'tool' && part.event) {
      hasToolPart = true;
      if (pendingConfirmation(part.event)) blocks.push({kind: 'confirmation', event: part.event});
      else if (toolEventDisplayName(part.event)) appendToolBlock(blocks, part.event);
    }
  }

  const fallbackText = fallbackAssistantText(message, streaming);
  const sourceEvents = Array.isArray(message?.events) ? message.events : [];
  const confirmations = sourceEvents.filter(pendingConfirmation);
  const fallbackEvents = sourceEvents.filter(event => !pendingConfirmation(event) && toolEventDisplayName(event));

  // 历史消息可能只有部分 parts。缺失字段只在对应类型完全不存在时回退，避免重复展示。
  if (!hideThinking && !hasReasoningPart && message?.reasoning) {
    blocks.unshift({kind: 'reasoning', text: String(message.reasoning)});
  }
  if (!hasToolPart && fallbackEvents.length) {
    insertBeforeFirstText(blocks, {kind: 'tools', events: fallbackEvents});
  }
  if (!hasTextPart && fallbackText) {
    blocks.push({kind: 'text', text: fallbackText});
  }
  for (const event of confirmations) {
    if (!blocks.some(block => block.kind === 'confirmation' && block.event === event)) {
      insertBeforeFirstText(blocks, {kind: 'confirmation', event});
    }
  }

  return {
    blocks,
    textParts: blocks.filter(block => block.kind === 'text').map(block => block.text),
  };
}

function reasoningPreview(text) {
  const value = String(text || '')
    .replace(/```[\s\S]*?```/g, '代码片段')
    .replace(/[`#>*_~]/g, '')
    .replace(/\s+/g, ' ')
    .trim();
  if (value.length <= 56) return value;
  return value.slice(0, 55).trimEnd() + '…';
}

export function executionBlockSummary(block, { streaming = false } = {}) {
  if (block?.kind === 'reasoning') {
    return {
      label: streaming ? '正在思考' : '思考过程',
      meta: reasoningPreview(block.text) || (streaming ? '执行中' : '点击查看详情'),
      tone: streaming ? 'running' : 'reasoning',
    };
  }

  const events = Array.isArray(block?.events) ? block.events : [];
  const failed = events.filter(event => event?.phase === 'error').length;
  const running = [...events].reverse().find(event => event?.phase === 'running');
  const names = [...new Set(events.map(toolEventDisplayName).filter(Boolean))];

  if (streaming && running) {
    return {
      label: `正在调用 ${toolEventDisplayName(running)}`,
      meta: events.length > 1 ? `本阶段 ${events.length} 项工具` : '执行中',
      tone: 'running',
    };
  }

  const label = failed
    ? '工具调用存在失败'
    : (events.length === 1 ? `调用 ${names[0] || '工具'}` : `调用 ${events.length} 项工具`);
  const nameSummary = names.slice(0, 2).join('、') + (names.length > 2 ? ' 等' : '');
  return {
    label,
    meta: [nameSummary, failed ? `${failed} 项失败` : ''].filter(Boolean).join(' · '),
    tone: failed ? 'error' : 'done',
  };
}
