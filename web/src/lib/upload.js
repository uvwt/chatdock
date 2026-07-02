// Browser upload transport. Kept outside App.jsx so attachment UI and API wiring stay separate.
export function uploadFileRequest(file, sessionID, authHeaders, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const form = new FormData();
    form.append('file', file);
    if (sessionID) form.append('session_id', sessionID);
    xhr.open('POST', '/api/files');
    const headers = authHeaders();
    Object.entries(headers || {}).forEach(([key, value]) => xhr.setRequestHeader(key, value));
    xhr.upload.onprogress = event => {
      if (!event.lengthComputable) return;
      onProgress?.(Math.max(1, Math.min(99, Math.round(event.loaded / event.total * 100))));
    };
    xhr.onload = () => {
      let data = {};
      try { data = JSON.parse(xhr.responseText || '{}'); } catch {}
      if (xhr.status >= 200 && xhr.status < 300) {
        onProgress?.(100);
        resolve(data);
      } else {
        const err = new Error(data.error || xhr.statusText || '上传失败');
        err.status = xhr.status;
        reject(err);
      }
    };
    xhr.onerror = () => reject(new Error('上传网络错误'));
    xhr.onabort = () => reject(new Error('上传已取消'));
    xhr.send(form);
  });
}
