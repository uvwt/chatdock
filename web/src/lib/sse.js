// Server-sent event stream parser for ChatDock streaming responses.
export class SSEStreamClosedError extends Error {
  constructor(lastEventID = 0) {
    super('流式连接在任务完成前中断');
    this.name = 'SSEStreamClosedError';
    this.code = 'SSE_STREAM_CLOSED';
    this.lastEventID = lastEventID;
  }
}

export async function readSSE(res, onEvent) {
  if (!res.body) throw new Error('流式响应没有可读取的响应体');

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let lastEventID = 0;
  let terminal = false;

  const dispatch = (block) => {
    const parsed = parseSSE(block);
    if (!parsed) return;
    if (parsed.id > 0) lastEventID = parsed.id;
    onEvent(parsed.event, parsed.data, parsed.id);
    terminal = terminal || isTerminalEvent(parsed.event, parsed.data);
  };

  while (true) {
    const {value, done} = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, {stream:true}).replaceAll('\r\n', '\n');
    const parts = buffer.split('\n\n');
    buffer = parts.pop() || '';
    for (const part of parts) dispatch(part);
  }

  buffer += decoder.decode().replaceAll('\r\n', '\n');
  if (buffer.trim()) dispatch(buffer);
  if (!terminal) throw new SSEStreamClosedError(lastEventID);
  return {lastEventID};
}

function parseSSE(block) {
  let event = 'message';
  let id = 0;
  const dataLines = [];
  for (const line of block.split('\n')) {
    if (!line || line.startsWith(':')) continue;
    if (line.startsWith('event:')) event = line.slice(6).trim();
    if (line.startsWith('id:')) id = Number.parseInt(line.slice(3).trim(), 10) || 0;
    if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart());
  }
  if (!dataLines.length) return null;
  return {event, id, data: JSON.parse(dataLines.join('\n'))};
}

function isTerminalEvent(event, data = {}) {
  if (event === 'done' || event === 'error') return true;
  return event === 'message_end' && (data.status === 'failed' || data.status === 'interrupted');
}
