import { fmtDuration, runStatusLabel } from './appUtils.js';
import { streamErrorMessage } from './chatPresentation.js';
import { appendToolStartEvent, mergeToolResultEvent } from './toolEvents.js';

const assistantEventNames = new Set([
  'tool_setup_ready',
  'tool_setup_error',
  'tool_call_start',
  'tool_call_result',
  'tool_confirmation_required',
  'tool_confirmation_resolved',
  'job_cancelled',
  'guidance_queued',
  'guidance_injected',
  'error',
]);

export function chatStreamStatsAfterEvent(stats, event, data = {}, paused = false) {
  switch (event) {
    case 'delta': {
      const chars = String(data.content || '').length + String(data.reasoning_content || '').length;
      return { ...stats, state: paused ? 'paused' : 'streaming', chars: stats.chars + chars };
    }
    case 'tool_setup_ready':
    case 'tool_call_result':
    case 'guidance_queued':
    case 'guidance_injected':
    case 'run_event':
      return { ...stats, events: stats.events + 1 };
    case 'tool_setup_error':
      return { ...stats, events: stats.events + 1, error: data.message || 'MCP 工具未接入' };
    case 'tool_call_start':
      return { ...stats, events: stats.events + 1, tools: stats.tools + 1 };
    case 'tool_confirmation_required':
      return { ...stats, events: stats.events + 1, state: 'paused' };
    case 'tool_confirmation_resolved':
      return { ...stats, events: stats.events + 1, state: 'streaming' };
    case 'job_cancelled':
      return { ...stats, events: stats.events + 1, state: 'stopping' };
    case 'message_end':
      if (data.status === 'failed') return { ...stats, state: 'error' };
      if (data.status === 'interrupted') return { ...stats, state: 'done' };
      return stats;
    case 'done':
      return { ...stats, state: 'done' };
    case 'error':
      return { ...stats, state: 'error', error: streamErrorMessage(data) };
    default:
      return stats;
  }
}

export function projectsChatStreamAssistant(event, data = {}) {
  if (assistantEventNames.has(event)) return true;
  return event === 'run_event' && data.kind !== 'tool_call' && data.kind !== 'tool_result';
}

export function chatStreamAssistantAfterEvent(message, event, data = {}) {
  switch (event) {
    case 'tool_setup_ready':
      return appendEvent(message, {
        kind: 'tool',
        text: data.mode === 'discovery'
          ? `已准备可用工具索引：${data.tool_count || 0} 个工具`
          : `MCP 已接入：${data.tool_count || 0} 个工具`,
        details: { event, data },
      });
    case 'tool_setup_error':
      return appendEvent(message, { kind: 'tool', text: `⚠️ MCP 未接入：${data.message || '工具初始化失败'}`, details: { event, data } });
    case 'tool_call_start':
      return appendToolStartEvent(message, event, data);
    case 'tool_call_result':
      return mergeToolResultEvent(message, event, data);
    case 'tool_confirmation_required':
      return appendEvent(message, {
        kind: 'confirm',
        text: `⏳ 等待确认工具：${data.tool || 'MCP 工具'}`,
        meta: '确认后模型会继续执行；拒绝则把拒绝结果返回给模型。',
        confirmation: data,
        status: 'pending',
        details: { event, tool: data.tool || '', arguments: data.arguments || {}, data },
      });
    case 'tool_confirmation_resolved':
      return {
        ...message,
        events: (message.events || []).map(item => item.confirmation?.id === data.id
          ? { ...item, status: 'resolved', text: `${data.approved ? '✅ 已允许工具：' : '⛔ 已拒绝工具：'}${data.tool || item.confirmation?.tool || 'MCP 工具'}` }
          : item),
      };
    case 'job_cancelled':
      return appendEvent(message, { kind: 'tool', text: '⏹️ 已请求停止生成', details: { event, data } });
    case 'guidance_queued':
      return appendEvent(message, { kind: 'guide', phase: 'running', text: '🧭 已收到引导，等待下一轮模型调用', meta: data.message || '', details: { event, data } });
    case 'guidance_injected':
      return appendEvent(message, { kind: 'guide', phase: 'done', text: '🧭 已将引导加入下一轮模型上下文', meta: data.message || '', details: { event, data } });
    case 'run_event': {
      const meta = [runStatusLabel(data.status || ''), data.server, data.action, fmtDuration(data.duration_ms)].filter(Boolean).join(' · ');
      return appendEvent(message, {
        kind: 'run',
        text: `🧭 ${data.summary || data.tool || 'MCP 工具事件'}`,
        meta,
        details: { event, tool: data.tool || '', arguments: data.arguments, result: data.result, error: data.error || '', duration_ms: data.duration_ms, data },
      });
    }
    case 'error': {
      const error = streamErrorMessage(data);
      return appendEvent({ ...message, error: data, answer: message.answer || '' }, { kind: 'error', phase: 'error', text: `⚠️ ${error}`, details: { event, error, data } });
    }
    default:
      return message;
  }
}

function appendEvent(message, event) {
  return { ...message, events: [...(message.events || []), event] };
}
