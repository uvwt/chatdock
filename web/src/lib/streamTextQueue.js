export function createStreamTextQueue() {
  const segments = [];
  let length = 0;

  function enqueue({ reasoning = '', content = '' } = {}) {
    appendSegment('reasoning', reasoning);
    appendSegment('text', content);
  }

  function appendSegment(kind, value) {
    const characters = Array.from(String(value || ''));
    if (!characters.length) return;
    const last = segments[segments.length - 1];
    if (last?.kind === kind) last.characters.push(...characters);
    else segments.push({ kind, characters });
    length += characters.length;
  }

  function take(maxCharacters) {
    let remaining = Math.max(0, Number(maxCharacters) || 0);
    const parts = [];
    while (remaining > 0 && segments.length) {
      const current = segments[0];
      const count = Math.min(remaining, current.characters.length);
      const text = current.characters.splice(0, count).join('');
      const lastPart = parts[parts.length - 1];
      if (lastPart?.kind === current.kind) lastPart.text += text;
      else parts.push({ kind: current.kind, text });
      length -= count;
      remaining -= count;
      if (!current.characters.length) segments.shift();
    }
    return parts;
  }

  function drain() {
    return take(length);
  }

  function clear() {
    segments.length = 0;
    length = 0;
  }

  return {
    enqueue,
    take,
    drain,
    clear,
    get length() { return length; },
  };
}

export function streamRevealCount(pendingCharacters) {
  const length = Math.max(0, Number(pendingCharacters) || 0);
  if (length > 240) return 8;
  if (length > 120) return 4;
  if (length > 48) return 2;
  return length > 0 ? 1 : 0;
}
