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

const maxUploadBytes = 32 << 20

const modelImageURLTTL = 6 * time.Hour

func (a *App) handleUploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("文件过大或表单不合法：%w", err))
		return
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
	if err := a.store.SaveAttachment(workspaceID, record); err != nil {
		if upload.OwnsStorage {
			_ = os.Remove(upload.StoragePath)
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, model.FileUploadResponse{Attachment: record.Attachment})
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

func (a *App) persistUploadedFile(workspaceID string, id string, name string, contentType string, source io.Reader) (persistedUpload, error) {
	uploadDir := filepath.Join(a.cfg.DataDir, "uploads", safeFileComponent(workspaceID))
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return persistedUpload{}, err
	}
	storagePath := filepath.Join(uploadDir, id+"_"+name)
	out, err := os.Create(storagePath)
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
		_ = os.Remove(storagePath)
		return persistedUpload{ID: id, StoragePath: blob.StoragePath, MIMEType: llm.FirstNonEmptyString(mimeType, blob.MIMEType), SHA256: sha, Size: blob.Size}, nil
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
	file, err := os.Open(record.StoragePath)
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
	file, err := os.Open(record.StoragePath)
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
