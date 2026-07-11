package chatdock

import (
	"compress/gzip"
	"errors"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	gzipWriter *gzip.Writer
	status     int
	enabled    bool
	decided    bool
	flushErr   error
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.decided {
		return
	}
	w.decided = true
	w.status = status
	if shouldGzipStatus(status) && shouldGzipContentType(w.Header().Get("Content-Type")) && w.Header().Get("Content-Encoding") == "" {
		w.enabled = true
		w.gzipWriter = gzip.NewWriter(w.ResponseWriter)
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Encoding", "gzip")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(data []byte) (int, error) {
	if !w.decided {
		w.WriteHeader(http.StatusOK)
	}
	if w.enabled && w.gzipWriter != nil {
		return w.gzipWriter.Write(data)
	}
	return w.ResponseWriter.Write(data)
}

func (w *gzipResponseWriter) Flush() {
	if w.enabled && w.gzipWriter != nil {
		if err := w.gzipWriter.Flush(); err != nil && w.flushErr == nil {
			w.flushErr = err
		}
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *gzipResponseWriter) Close() error {
	if w.enabled && w.gzipWriter != nil {
		return errors.Join(w.flushErr, w.gzipWriter.Close())
	}
	return w.flushErr
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		if r.Method == http.MethodHead || !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || isStreamingRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipResponseWriter{ResponseWriter: w}
		defer func() {
			if err := gw.Close(); err != nil {
				logError("gzip_response_close_failed", err, logFields{"request_id": requestIDFromRequest(r), "path": r.URL.Path})
			}
		}()
		next.ServeHTTP(gw, r)
	})
}

func shouldGzipStatus(status int) bool {
	return status >= 200 && status != http.StatusNoContent && status != http.StatusNotModified && status < 300
}

func shouldGzipContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	return strings.HasPrefix(contentType, "application/json") ||
		strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "javascript") ||
		strings.Contains(contentType, "manifest") ||
		strings.HasPrefix(contentType, "image/svg")
}

func isStreamingRoute(path string) bool {
	return path == "/api/chat/stream" || (strings.HasPrefix(path, "/api/chat/jobs/") && strings.HasSuffix(path, "/events"))
}
