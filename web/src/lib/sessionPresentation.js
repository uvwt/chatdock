export function sessionRowID(row) {
  return String(row?.session_id || row?.id || '').trim();
}

export function scheduledTaskSessionRows(runs = []) {
  const seen = new Set();
  return runs.filter(run => {
    const id = String(run?.session_id || '').trim();
    if (!id || seen.has(id)) return false;
    seen.add(id);
    return true;
  });
}

export function unpinnedSessionRows(rows = [], pinnedSessions = []) {
  const pinnedIDs = new Set((Array.isArray(pinnedSessions) ? pinnedSessions : []).map(sessionRowID).filter(Boolean));
  return (Array.isArray(rows) ? rows : []).filter(row => {
    const id = sessionRowID(row);
    return id && !row?.pinned && !pinnedIDs.has(id);
  });
}

export function visibleSessionRows({ sessionSearch = '', sessionSearchResults = [], sessions = [] }) {
  return String(sessionSearch || '').trim() ? sessionSearchResults : sessions;
}
