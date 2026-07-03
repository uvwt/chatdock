package chatdock

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

const maxUploadBytes = 32 << 20

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

	name := cleanUploadName(header.Filename)
	id := model.NewID()
	sessionID := strings.TrimSpace(r.FormValue("session_id"))
	prompt := a.store.ActivePrompt()
	uploadDir := filepath.Join(a.cfg.DataDir, "uploads", safeFileComponent(prompt))
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	storagePath := filepath.Join(uploadDir, id+"_"+name)
	out, err := os.Create(storagePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	hash := sha256.New()
	written, copyErr := io.Copy(out, io.TeeReader(file, hash))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(storagePath)
		writeError(w, http.StatusInternalServerError, copyErr)
		return
	}
	if closeErr != nil {
		_ = os.Remove(storagePath)
		writeError(w, http.StatusInternalServerError, closeErr)
		return
	}
	if written <= 0 {
		_ = os.Remove(storagePath)
		writeError(w, http.StatusBadRequest, fmt.Errorf("文件为空"))
		return
	}

	mimeType := llm.FirstNonEmptyString(header.Header.Get("Content-Type"), mime.TypeByExtension(strings.ToLower(filepath.Ext(name))), "application/octet-stream")
	text, status, extractErr := extractAttachmentText(storagePath, name, mimeType)
	if extractErr != nil && strings.TrimSpace(text) == "" {
		status = "stored"
	}
	record := model.AttachmentRecord{
		Attachment: model.Attachment{
			ID:        id,
			Name:      name,
			MIMEType:  mimeType,
			Size:      written,
			Status:    status,
			HasText:   strings.TrimSpace(text) != "",
			TextBytes: len([]byte(text)),
			CreatedAt: time.Now(),
		},
		Prompt:      prompt,
		SessionID:   sessionID,
		StoragePath: storagePath,
		SHA256:      hex.EncodeToString(hash.Sum(nil)),
		TextContent: text,
	}
	if err := a.store.SaveAttachment(record); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSONResponse(w, http.StatusOK, model.FileUploadResponse{Attachment: record.Attachment})
}
