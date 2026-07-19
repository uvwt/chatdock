package chatdock

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

const (
	maxUploadBytes     = 32 << 20
	maxMultipartMemory = 1 << 20
	uploadDirMode      = 0o700
	uploadFileMode     = 0o600
)

const modelImageURLTTL = 6 * time.Hour

func (a *App) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		status := http.StatusBadRequest
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			status = http.StatusRequestEntityTooLarge
		}
		writeError(w, status, fmt.Errorf("文件过大或表单不合法：%w", err))
		return
	}
	if r.MultipartForm != nil {
		defer func() {
			if err := r.MultipartForm.RemoveAll(); err != nil {
				logError("multipart_cleanup_failed", err, logFields{"request_id": requestIDFromRequest(r)})
			}
		}()
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("缺少 file 字段：%w", err))
		return
	}
	defer file.Close()

	workspaceID := a.workspaceIDFromRequest(r)
	name := cleanUploadName(header.Filename)
	upload, err := a.persistUploadedFile(workspaceID, model.NewID(), name, header.Header.Get("Content-Type"), file)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errEmptyUpload) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err)
		return
	}
	text, status, extractErr := extractAttachmentText(upload.StoragePath, name, upload.MIMEType)
	if extractErr != nil && strings.TrimSpace(text) == "" {
		status = "stored"
	}
	record := model.AttachmentRecord{
		Attachment: model.Attachment{
			ID:        upload.ID,
			Name:      name,
			MIMEType:  upload.MIMEType,
			Size:      upload.Size,
			Status:    status,
			HasText:   strings.TrimSpace(text) != "",
			TextBytes: len([]byte(text)),
			CreatedAt: time.Now(),
		},
		Prompt:      workspaceID,
		SessionID:   strings.TrimSpace(r.FormValue("session_id")),
		StoragePath: upload.StoragePath,
		SHA256:      upload.SHA256,
		TextContent: text,
	}
	saved, err := a.store.SaveAttachment(workspaceID, record)
	if err != nil {
		if upload.OwnsStorage {
			_ = os.Remove(upload.StoragePath)
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if upload.OwnsStorage && saved.StoragePath != upload.StoragePath {
		if err := os.Remove(upload.StoragePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			logError("duplicate_upload_cleanup_failed", err, logFields{"path": upload.StoragePath, "sha256": upload.SHA256})
		}
	}
	writeJSONResponse(w, http.StatusOK, model.FileUploadResponse{Attachment: saved.Attachment})
}

var errEmptyUpload = errors.New("文件为空")

type persistedUpload struct {
	ID          string
	StoragePath string
	MIMEType    string
	SHA256      string
	Size        int64
	OwnsStorage bool
}

func resolveAttachmentStoragePath(dataDir string, storedPath string) (string, error) {
	root, err := filepath.Abs(strings.TrimSpace(dataDir))
	if err != nil {
		return "", err
	}
	storedPath = strings.TrimSpace(storedPath)
	if storedPath == "" {
		return "", fmt.Errorf("attachment storage path is empty")
	}
	cleaned := filepath.Clean(storedPath)
	if !filepath.IsAbs(cleaned) {
		if !isSafeRelativePath(cleaned) {
			return "", fmt.Errorf("attachment storage path escapes data directory")
		}
		absolute, absErr := filepath.Abs(cleaned)
		if absErr != nil {
			return "", absErr
		}
		if rel, relErr := filepath.Rel(root, absolute); relErr == nil && isSafeRelativePath(rel) {
			return absolute, nil
		}
		return joinAttachmentDataPath(root, cleaned)
	}
	if rel, relErr := filepath.Rel(root, cleaned); relErr == nil && isSafeRelativePath(rel) {
		return cleaned, nil
	}
	parts := strings.Split(filepath.ToSlash(cleaned), "/")
	for index := len(parts) - 1; index >= 0; index-- {
		if parts[index] != "uploads" {
			continue
		}
		return joinAttachmentDataPath(root, filepath.FromSlash(strings.Join(parts[index:], "/")))
	}
	return "", fmt.Errorf("attachment storage path is outside data directory")
}

func joinAttachmentDataPath(root string, relative string) (string, error) {
	relative = filepath.Clean(relative)
	if !isSafeRelativePath(relative) || strings.Split(filepath.ToSlash(relative), "/")[0] != "uploads" {
		return "", fmt.Errorf("attachment storage path must be under uploads")
	}
	resolved := filepath.Join(root, relative)
	if rel, err := filepath.Rel(root, resolved); err != nil || !isSafeRelativePath(rel) {
		return "", fmt.Errorf("attachment storage path escapes data directory")
	}
	return resolved, nil
}

func isSafeRelativePath(path string) bool {
	return path != "" && path != "." && path != ".." && !filepath.IsAbs(path) && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func (a *App) persistUploadedFile(workspaceID string, id string, name string, contentType string, source io.Reader) (persistedUpload, error) {
	uploadDir := filepath.Join(a.cfg.DataDir, "uploads", safeFileComponent(workspaceID))
	if err := os.MkdirAll(uploadDir, uploadDirMode); err != nil {
		return persistedUpload{}, err
	}
	storagePath := filepath.Join(uploadDir, id+"_"+name)
	out, err := os.OpenFile(storagePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, uploadFileMode)
	if err != nil {
		return persistedUpload{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(out, io.TeeReader(source, hash))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(storagePath)
		return persistedUpload{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(storagePath)
		return persistedUpload{}, closeErr
	}
	if written <= 0 {
		_ = os.Remove(storagePath)
		return persistedUpload{}, errEmptyUpload
	}

	mimeType := llm.FirstNonEmptyString(contentType, mime.TypeByExtension(strings.ToLower(filepath.Ext(name))), "application/octet-stream")
	sha := hex.EncodeToString(hash.Sum(nil))
	blob, ok, err := a.store.AttachmentBlobBySHA256(sha)
	if err != nil {
		_ = os.Remove(storagePath)
		return persistedUpload{}, err
	}
	if ok && strings.TrimSpace(blob.StoragePath) != "" {
		blobPath, resolveErr := resolveAttachmentStoragePath(a.cfg.DataDir, blob.StoragePath)
		if resolveErr != nil {
			_ = os.Remove(storagePath)
			return persistedUpload{}, resolveErr
		}
		info, statErr := os.Stat(blobPath)
		switch {
		case statErr == nil && !info.IsDir() && (blob.Size <= 0 || info.Size() == blob.Size):
			_ = os.Remove(storagePath)
			return persistedUpload{ID: id, StoragePath: blob.StoragePath, MIMEType: llm.FirstNonEmptyString(mimeType, blob.MIMEType), SHA256: sha, Size: blob.Size}, nil
		case statErr != nil && !errors.Is(statErr, os.ErrNotExist):
			_ = os.Remove(storagePath)
			return persistedUpload{}, statErr
		case blob.RefCount > 0:
			_ = os.Remove(storagePath)
			return persistedUpload{}, fmt.Errorf("attachment blob file is missing or invalid: %s", blob.StoragePath)
		}
		// 零引用 Blob 只是可复用缓存；文件已丢失时保留本次新文件，
		// SaveAttachment 会让它在同一事务中接管 canonical path。
	}
	return persistedUpload{ID: id, StoragePath: storagePath, MIMEType: mimeType, SHA256: sha, Size: written, OwnsStorage: true}, nil
}

func (a *App) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	record, err := a.store.AttachmentRecordByID(a.workspaceIDFromRequest(r), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	storagePath, err := resolveAttachmentStoragePath(a.cfg.DataDir, record.StoragePath)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	file, err := os.Open(storagePath)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 只从当前工作空间的附件记录取真实路径；前端点击卡片时通过带鉴权的 fetch 请求此接口。
	w.Header().Set("Content-Type", llm.FirstNonEmptyString(record.MIMEType, "application/octet-stream"))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": record.Name}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, record.Name, stat.ModTime(), file)
}

func (a *App) handleModelImageFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	expires := strings.TrimSpace(r.URL.Query().Get("expires"))
	sig := strings.TrimSpace(r.URL.Query().Get("sig"))
	if !a.verifyModelImageSignature(id, expires, sig, time.Now()) {
		writeError(w, http.StatusForbidden, fmt.Errorf("invalid or expired image link"))
		return
	}

	record, err := a.store.AttachmentRecordByID(a.workspaceIDFromRequest(r), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if !model.IsImageAttachment(record) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("attachment is not an image"))
		return
	}
	storagePath, err := resolveAttachmentStoragePath(a.cfg.DataDir, record.StoragePath)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	file, err := os.Open(storagePath)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", llm.FirstNonEmptyString(record.MIMEType, "application/octet-stream"))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": record.Name}))
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, record.Name, stat.ModTime(), file)
}

func (a *App) prepareVisionAttachmentURLs(history []model.Message) []model.Message {
	base := strings.TrimSpace(a.cfg.PublicBaseURL)
	if base == "" {
		return history
	}
	expiresAt := time.Now().Add(modelImageURLTTL)
	out := cloneModelMessages(history)
	for msgIndex := range out {
		for attachmentIndex := range out[msgIndex].ModelAttachments {
			attachment := &out[msgIndex].ModelAttachments[attachmentIndex]
			if !model.IsImageAttachment(*attachment) {
				continue
			}
			modelURL, err := a.modelImageURL(attachment.ID, attachment.Name, expiresAt)
			if err != nil {
				continue
			}
			attachment.ModelURL = modelURL
		}
	}
	return out
}

func cloneModelMessages(history []model.Message) []model.Message {
	out := make([]model.Message, len(history))
	copy(out, history)
	for i := range out {
		out[i].Attachments = append([]model.Attachment(nil), history[i].Attachments...)
		out[i].ModelAttachments = append([]model.AttachmentRecord(nil), history[i].ModelAttachments...)
		out[i].Parts = append([]model.MessagePart(nil), history[i].Parts...)
		out[i].Events = append([]model.MessageEvent(nil), history[i].Events...)
	}
	return out
}

func (a *App) modelImageURL(id string, filename string, expiresAt time.Time) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("attachment id is empty")
	}
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(a.cfg.PublicBaseURL), "/"))
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("CHATDOCK_PUBLIC_BASE_URL must use http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("CHATDOCK_PUBLIC_BASE_URL host is empty")
	}
	expires := strconv.FormatInt(expiresAt.Unix(), 10)
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/model-images/" + url.PathEscape(id) + "/" + url.PathEscape(modelImageURLName(filename))
	q := parsed.Query()
	q.Set("expires", expires)
	q.Set("sig", a.signModelImage(id, expires))
	parsed.RawQuery = q.Encode()
	return parsed.String(), nil
}

func modelImageURLName(filename string) string {
	name := cleanUploadName(filename)
	if strings.TrimSpace(name) == "" {
		return "image"
	}
	if filepath.Ext(name) == "" {
		return name + ".jpg"
	}
	return name
}

func (a *App) verifyModelImageSignature(id string, expiresRaw string, sig string, now time.Time) bool {
	id = strings.TrimSpace(id)
	expiresRaw = strings.TrimSpace(expiresRaw)
	sig = strings.TrimSpace(sig)
	if id == "" || expiresRaw == "" || sig == "" {
		return false
	}
	expires, err := strconv.ParseInt(expiresRaw, 10, 64)
	if err != nil || expires <= now.Unix() {
		return false
	}
	expected := a.signModelImage(id, expiresRaw)
	return hmac.Equal([]byte(sig), []byte(expected))
}

func (a *App) signModelImage(id string, expires string) string {
	mac := hmac.New(sha256.New, a.modelImageSigningSecret())
	_, _ = mac.Write([]byte(strings.TrimSpace(id)))
	_, _ = mac.Write([]byte("\n"))
	_, _ = mac.Write([]byte(strings.TrimSpace(expires)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *App) modelImageSigningSecret() []byte {
	if token := strings.TrimSpace(a.cfg.AuthToken); token != "" {
		return []byte(token)
	}
	fallback := "chatdock-model-image:" + strings.TrimSpace(a.cfg.DataDir)
	if fallback == "chatdock-model-image:" {
		fallback = "chatdock-model-image:default"
	}
	sum := sha256.Sum256([]byte(fallback))
	return sum[:]
}
