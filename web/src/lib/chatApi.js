import { readSSE } from './sse.js';

export function fetchChatJobs(api, sessionID) {
  return api('/api/chat/jobs?session_id=' + encodeURIComponent(sessionID));
}

export async function streamChatJobEvents({jobID, authHeaders, signal, onEvent}) {
  const res = await fetch('/api/chat/jobs/' + encodeURIComponent(jobID) + '/events?after=0', {headers: authHeaders(), signal});
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || res.statusText);
  }
  await readSSE(res, onEvent);
}

export async function streamChat({authHeaders, signal, sessionID, message, attachmentIDs, onEvent}) {
  const res = await fetch('/api/chat/stream', {
    method:'POST',
    headers: authHeaders({'Content-Type':'application/json'}),
    body: JSON.stringify({session_id: sessionID, message, attachment_ids: attachmentIDs}),
    signal,
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw new Error(data.error || res.statusText);
  }
  await readSSE(res, onEvent);
}
