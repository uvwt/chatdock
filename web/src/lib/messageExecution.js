function fallbackAssistantText(message, streaming) {
  if (streaming) return String(message?.answer || '');
  return String(message?.content || message?.answer || '');
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

export function splitAssistantMessage(message, { streaming = false, hideThinking = false } = {}) {
  const parts = Array.isArray(message?.parts) ? message.parts : [];
  const partTexts = parts.filter(part => part?.kind === 'text' && part.text).map(part => String(part.text));
  const partEvents = parts.filter(part => part?.kind === 'tool' && part.event).map(part => part.event);
  const partReasoning = parts.filter(part => part?.kind === 'reasoning' && part.text).map(part => String(part.text));
  const textParts = partTexts.length ? partTexts : [fallbackAssistantText(message, streaming)].filter(Boolean);
  const sourceEvents = partEvents.length ? partEvents : (Array.isArray(message?.events) ? message.events : []);
  const confirmations = sourceEvents.filter(event => event?.kind === 'confirm' && event.status !== 'resolved');
  const executionEvents = sourceEvents.filter(event => {
    if (event?.kind === 'confirm' && event.status !== 'resolved') return false;
    return !!toolEventDisplayName(event);
  });
  const reasoningParts = hideThinking
    ? []
    : (partReasoning.length ? partReasoning : [String(message?.reasoning || '')].filter(Boolean));

  return { textParts, executionEvents, confirmations, reasoningParts };
}

export function executionSummary({ events = [], reasoningParts = [], streaming = false } = {}) {
  const failed = events.filter(event => event?.phase === 'error').length;
  const running = [...events].reverse().find(event => event?.phase === 'running');
  const completed = events.filter(event => event?.phase !== 'running' && event?.phase !== 'error').length;

  if (streaming && running) {
    return {
      label: `正在调用 ${toolEventDisplayName(running)}`,
      meta: completed ? `已完成 ${completed} 项` : '执行中',
      tone: 'running',
    };
  }
  if (streaming && !events.length && reasoningParts.length) {
    return { label: '正在思考', meta: '执行中', tone: 'running' };
  }

  const details = [];
  if (events.length) details.push(`${events.length} 项执行记录`);
  if (reasoningParts.length) details.push(`${reasoningParts.length} 段思考`);
  if (failed) details.push(`${failed} 项失败`);
  return {
    label: failed ? '执行过程存在失败' : '执行过程',
    meta: details.join(' · '),
    tone: failed ? 'error' : 'done',
  };
}
