function hasDialogValue(value) {
  if (value == null || value === '') return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value).length > 0;
  return true;
}

function safeJSONStringify(value) {
  try { return JSON.stringify(value, null, 2); }
  catch { return String(value || ''); }
}


function targetToolName(data) {
  if (data?.arguments?._parse_error) return '参数 JSON 解析失败';
  return data?.arguments?.name || data?.result?.tool || data?.tool || '工具';
}

function toolEventText(phase, data = {}) {
  const tool = data.tool || '';
  if (tool === 'chatdock_tools_search') return phase === 'start' ? '正在查找可用工具' : (data.ok === false ? '查找可用工具失败' : '已查找可用工具');
  if (tool === 'chatdock_tools_describe') return phase === 'start' ? '正在查看工具详情' : (data.ok === false ? '查看工具详情失败' : '已查看工具详情');
  if (tool === 'chatdock_tool_execute') {
    const target = targetToolName(data);
    if (phase === 'start') return '正在调用工具：' + target;
    return data.ok === false ? '调用工具失败：' + target : '已调用工具：' + target;
  }
  if (phase === 'start') return '正在调用：' + (tool || '工具');
  return (data.ok ? '调用完成：' : '调用失败：') + (tool || '工具');
}

function toolCallKey(data = {}) {
  const args = data.arguments || {};
  return [data.tool || '', safeJSONStringify(args)].join('::');
}

function toolEventMeta(data = {}) {
  const query = data.arguments?.query || data.result?.query;
  if (data.tool === 'chatdock_tools_search' && query) return '关键词：' + query;
  const target = data.arguments?.name || data.result?.tool;
  if ((data.tool === 'chatdock_tools_describe' || data.tool === 'chatdock_tool_execute') && target) return target;
  return '';
}

export function appendInlineTextPart(message, text) {
  const parts = [...(message.parts || [])];
  const last = parts[parts.length - 1];
  if (last?.kind === 'text') parts[parts.length - 1] = {...last, text: (last.text || '') + text};
  else parts.push({kind: 'text', text});
  return {...message, answer: (message.answer || '') + text, parts};
}

export function appendInlineReasoningPart(message, text) {
  const parts = [...(message.parts || [])];
  const last = parts[parts.length - 1];
  if (last?.kind === 'reasoning') parts[parts.length - 1] = {...last, text: (last.text || '') + text};
  else parts.push({kind: 'reasoning', text});
  return {...message, reasoning: (message.reasoning || '') + text, parts};
}

function eventHasDisplayName(item = {}) {
  return !!(item.details?.arguments?.name || item.details?.data?.arguments?.name || item.details?.data?.result?.tool || item.details?.tool || item.details?.data?.tool);
}

function appendEventPart(message, item) {
  const events = [...(message.events || []), item];
  const parts = eventHasDisplayName(item) ? [...(message.parts || []), {kind: 'tool', callKey: item.callKey || '', event: item}] : (message.parts || []);
  return {...message, events, parts};
}

export function appendToolStartEvent(message, event, data) {
  return appendEventPart(message, {
    kind: 'tool',
    phase: 'running',
    callKey: toolCallKey(data),
    text: toolEventText('start', data),
    meta: toolEventMeta(data),
    details: {event, tool: data.tool || '', arguments: data.arguments || {}, data},
  });
}

export function mergeToolResultEvent(message, event, data) {
  const key = toolCallKey(data);
  const events = [...(message.events || [])];
  const parts = [...(message.parts || [])];
  const hasArguments = Object.keys(data.arguments || {}).length > 0;
  const sameRunningEvent = item => {
    if (item.kind !== 'tool' || item.phase !== 'running') return false;
    if (item.callKey === key) return true;
    return !hasArguments && item.details?.tool === data.tool;
  };
  const buildResultEvent = previousEvent => {
    const previousDetails = previousEvent?.details || {};
    const previousData = previousDetails.data && typeof previousDetails.data === 'object' ? previousDetails.data : {};
    const mergedArguments = hasArguments ? data.arguments : (previousDetails.arguments || previousData.arguments || data.arguments || {});
    const mergedData = { ...previousData, ...data };
    if (hasDialogValue(mergedArguments)) mergedData.arguments = mergedArguments;
    return {
      kind: 'tool',
      phase: data.ok ? 'done' : 'error',
      callKey: previousEvent?.callKey || key,
      text: toolEventText('result', data),
      meta: toolEventMeta(mergedData),
      details: {
        ...previousDetails,
        event,
        tool: data.tool || previousDetails.tool || '',
        ok: !!data.ok,
        arguments: mergedArguments,
        result: data.result,
        error: data.error || '',
        data: mergedData,
      },
    };
  };
  const index = events.findLastIndex(sameRunningEvent);
  const nextEvent = buildResultEvent(index >= 0 ? events[index] : null);
  if (index >= 0) events[index] = {...events[index], ...nextEvent};
  else events.push(nextEvent);
  const partIndex = parts.findLastIndex(part => part.kind === 'tool' && sameRunningEvent(part.event || {}));
  if (partIndex >= 0) {
    const mergedPartEvent = buildResultEvent(parts[partIndex].event || null);
    parts[partIndex] = {...parts[partIndex], event: {...parts[partIndex].event, ...mergedPartEvent}};
  } else if (eventHasDisplayName(nextEvent)) parts.push({kind: 'tool', callKey: nextEvent.callKey, event: nextEvent});
  return {...message, events, parts};
}
