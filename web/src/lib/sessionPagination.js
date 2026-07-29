export const SESSION_PAGE_SIZE = 30;

export function normalizeSessionPage(data) {
  if (!data || Array.isArray(data) || !Array.isArray(data.sessions)) {
    throw new TypeError('invalid session page response');
  }
  return {
    sessions: data.sessions,
    nextCursor: String(data.next_cursor || ''),
    hasMore: !!data.has_more,
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
  return {
    id: session.id,
    title: session.title || '新会话',
    pinned: !!session.pinned,
    project_id: session.project_id || '',
    updated_at: session.updated_at,
  };
}

export function sessionMatchesProjectFilter(session, projectFilter = 'all') {
  const filter = String(projectFilter || 'all').trim() || 'all';
  const projectID = String(session?.project_id || '').trim();
  if (filter === 'all') return true;
  if (filter === 'plain') return !projectID;
  return projectID === filter;
}

export function upsertSessionSummary(items, session, { requirePinned } = {}) {
  const summary = sessionSummaryFromSession(session);
  if (!summary) return items || [];
  const rest = (items || []).filter(item => item.id !== summary.id);
  if (requirePinned === true && !summary.pinned) return rest;
  if (requirePinned === false && summary.pinned) return rest;
  return mergeSessionPages(rest, [summary]).sort(compareSessionSummaries);
}

export function removeSessionSummary(items, sessionID) {
  const id = String(sessionID || '').trim();
  if (!id) return items || [];
  return (items || []).filter(item => item.id !== id);
}

function compareSessionSummaries(left, right) {
  if (!!left.pinned !== !!right.pinned) return left.pinned ? -1 : 1;
  const updated = String(right.updated_at || '').localeCompare(String(left.updated_at || ''));
  if (updated) return updated;
  return String(right.id || '').localeCompare(String(left.id || ''));
}
