import { readSSE } from './sse.js';

export function fetchChatJobs(api, sessionID) {
  return api('/api/chat/jobs?session_id=' + encodeURIComponent(sessionID));
}

export function cancelChatJob(api, jobID) {
  return api('/api/chat/jobs/' + encodeURIComponent(jobID) + '/cancel', {method:'POST'});
}

export function resolveMCPConfirmation(api, id, approve) {
  return api('/api/mcp/confirmations/' + encodeURIComponent(id) + '/resolve', {method:'POST', body: JSON.stringify({approve: !!approve})});
}

export async function streamChatJobEvents({jobID, authHeaders, signal, onEvent}) {
  const res = await fetch('/api/chat/jobs/' + encodeURIComponent(jobID) + '/events?after=0', {headers: authHeaders(), signal});
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw requestError(res, data);
  }
  await readSSE(res, onEvent);
}

export async function streamChat({authHeaders, signal, sessionID, message, attachmentIDs, providerID, model, regenerate = false, onEvent}) {
  const res = await fetch('/api/chat/stream', {
    method:'POST',
    headers: authHeaders({'Content-Type':'application/json'}),
    body: JSON.stringify({session_id: sessionID, message, attachment_ids: attachmentIDs, provider_id: providerID || '', model: model || '', regenerate: !!regenerate}),
    signal,
  });
  if (!res.ok) {
    const data = await res.json().catch(() => ({}));
    throw requestError(res, data);
  }
  await readSSE(res, onEvent);
}


function requestError(res, data = {}) {
  const requestID = res.headers.get('X-Request-ID') || data.request_id || '';
  const message = data.error || data.message || res.statusText || '请求失败';
  const err = new Error(message);
  err.request_id = requestID;
  err.status = res.status;
  return err;
}
