export function createJsonApi({authHeaders, onUnauthorized} = {}) {
  return async function api(path, opt = {}) {
    const extraHeaders = opt.headers || {};
    const headers = authHeaders ? authHeaders({'Content-Type':'application/json', ...extraHeaders}) : {'Content-Type':'application/json', ...extraHeaders};
    const res = await fetch(path, {...opt, headers});
    const data = await res.json().catch(() => ({}));
    if (!res.ok) {
      const err = new Error(data.error || res.statusText);
      err.status = res.status;
      err.path = path;
      if (res.status === 401 && path !== '/api/auth/login' && onUnauthorized) onUnauthorized(err);
      throw err;
    }
    return data;
  };
}

export async function fetchWithAuth(path, {authHeaders, ...opt} = {}) {
  const headers = authHeaders ? authHeaders(opt.headers || {}) : (opt.headers || {});
  const res = await fetch(path, {...opt, headers});
  if (!res.ok) throw new Error(await res.text() || res.statusText);
  return res;
}
