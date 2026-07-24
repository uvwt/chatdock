export function scheduledTaskSessionRows(runs = []) {
  const seen = new Set();
  return runs.filter(run => {
    const id = String(run?.session_id || '').trim();
    if (!id || seen.has(id)) return false;
    seen.add(id);
    return true;
  });
}

export function visibleSessionRows({ sessionSearch = '', sessionSearchResults = [], sessions = [] }) {
  return String(sessionSearch || '').trim() ? sessionSearchResults : sessions;
}
