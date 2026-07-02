// Server-sent event stream parser for ChatDock streaming responses.
export async function readSSE(res, onEvent) {
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const {value, done} = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, {stream:true});
    const parts = buffer.split('\n\n');
    buffer = parts.pop() || '';
    for (const part of parts) {
      const event = parseSSE(part);
      if (event) onEvent(event.event, event.data);
    }
  }
  if (buffer.trim()) {
    const event = parseSSE(buffer);
    if (event) onEvent(event.event, event.data);
  }
}

function parseSSE(block) {
  let event = 'message';
  const dataLines = [];
  for (const line of block.split('\n')) {
    if (line.startsWith('event:')) event = line.slice(6).trim();
    if (line.startsWith('data:')) dataLines.push(line.slice(5).trim());
  }
  if (!dataLines.length) return null;
  return {event, data: JSON.parse(dataLines.join('\n'))};
}
