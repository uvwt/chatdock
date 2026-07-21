export const SESSION_PAGE_SIZE = 30;

export function normalizeSessionPage(data) {
  if (Array.isArray(data)) {
    return { sessions: data, nextCursor: '', hasMore: false };
  }
  return {
    sessions: Array.isArray(data?.sessions) ? data.sessions : [],
    nextCursor: String(data?.next_cursor || ''),
    hasMore: !!data?.has_more,
  };
}

export function mergeSessionPages(current = [], incoming = []) {
  const byID = new Map();
  for (const session of [...current, ...incoming]) {
    if (session?.id) byID.set(session.id, session);
  }
  return Array.from(byID.values());
}

export function sessionSummaryFromSession(session) {
  if (!session?.id) return null;
  const messages = Array.isArray(session.messages) ? session.messages : [];
  const last = messages[messages.length - 1] || {};
  let preview = String(last.content || last.error?.message || '').replace(/\s+/g, ' ').trim();
  if ([...preview].length > 120) preview = [...preview].slice(0, 120).join('') + '…';
  return {
    id: session.id,
    title: session.title || '新会话',
    pinned: !!session.pinned,
    provider_id: session.provider_id || '',
    model: session.model || '',
    preview,
    last_role: last.role || '',
    created_at: session.created_at,
    updated_at: session.updated_at,
    count: messages.length,
  };
}

export function upsertSessionSummary(items, session) {
  const summary = sessionSummaryFromSession(session);
  if (!summary) return items || [];
  const merged = mergeSessionPages((items || []).filter(item => item.id !== summary.id), [summary]);
  return merged.sort(compareSessionSummaries);
}

function compareSessionSummaries(left, right) {
  if (!!left.pinned !== !!right.pinned) return left.pinned ? -1 : 1;
  const updated = String(right.updated_at || '').localeCompare(String(left.updated_at || ''));
  if (updated) return updated;
  return String(right.id || '').localeCompare(String(left.id || ''));
}
