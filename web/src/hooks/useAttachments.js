import { useCallback, useMemo, useState } from 'react';
import { uploadFileRequest } from '../lib/upload.js';

export function useAttachments({ authHeaders, busy, setAuthPage, showToast }) {
  const [pendingAttachments, setPendingAttachments] = useState([]);
  const [uploadingFiles, setUploadingFiles] = useState(false);

  const readyAttachments = useMemo(() => pendingAttachments.filter(item => item.id && !item.uploading && !item.error && !String(item.id).startsWith('local_')), [pendingAttachments]);
  const pendingAttachmentIDs = useMemo(() => readyAttachments.map(item => item.id), [readyAttachments]);

  const clearAttachments = useCallback(() => {
    setPendingAttachments([]);
  }, []);

  const removePendingAttachment = useCallback((id) => {
    setPendingAttachments(prev => prev.filter(item => item.id !== id));
  }, []);

  const downloadAttachment = useCallback(async (item) => {
    if (!item?.id || String(item.id).startsWith('local_')) return;
    try {
      const response = await fetch('/api/files/' + encodeURIComponent(item.id), { headers: authHeaders() });
      if (!response.ok) throw new Error('HTTP ' + response.status);
      const blob = await response.blob();
      const url = URL.createObjectURL(blob);
      const link = document.createElement('a');
      link.href = url;
      link.download = item.name || 'attachment';
      document.body.appendChild(link);
      link.click();
      link.remove();
      window.setTimeout(() => URL.revokeObjectURL(url), 1000);
    } catch (e) {
      showToast('附件下载失败：' + e.message, 'error');
    }
  }, [authHeaders, showToast]);

  const handleFileSelect = useCallback(async (event, { current, createPersistedSession }) => {
    const files = Array.from(event.target.files || []);
    event.target.value = '';
    if (!files.length) return;
    if (busy) {
      showToast('当前回复还在进行中，请稍后再上传。', 'error');
      return;
    }
    let sessionID = current;
    if (!sessionID) {
      const s = await createPersistedSession();
      if (!s) return;
      sessionID = s.id;
    }
    setUploadingFiles(true);
    try {
      for (const file of files) {
        const localID = 'local_' + Date.now() + '_' + Math.random().toString(16).slice(2);
        setPendingAttachments(prev => [...prev, { id: localID, name: file.name || 'upload', size: file.size || 0, mime_type: file.type || 'application/octet-stream', status: 'uploading', uploading: true, progress: 0 }]);
        try {
          const data = await uploadFileRequest(file, sessionID, authHeaders, progress => {
            setPendingAttachments(prev => prev.map(item => item.id === localID ? { ...item, progress } : item));
          });
          setPendingAttachments(prev => prev.map(item => item.id === localID ? { ...data.attachment, progress: 100 } : item));
        } catch (e) {
          if (e.status === 401) setAuthPage(e);
          setPendingAttachments(prev => prev.map(item => item.id === localID ? { ...item, uploading: false, error: e.message || '上传失败', status: 'failed' } : item));
          showToast('上传失败：' + (e.message || '未知错误'), 'error');
        }
      }
    } finally {
      setUploadingFiles(false);
    }
  }, [authHeaders, busy, setAuthPage, showToast]);

  return {
    pendingAttachments,
    pendingAttachmentIDs,
    readyAttachments,
    uploadingFiles,
    clearAttachments,
    downloadAttachment,
    handleFileSelect,
    removePendingAttachment,
  };
}
