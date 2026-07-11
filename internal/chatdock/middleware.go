package chatdock

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func isPublicBackendRoute(requestPath string) bool {
	return requestPath == "/api/auth/status" || requestPath == "/api/auth/login" || strings.HasPrefix(requestPath, "/api/model-images/")
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(data)
	r.bytes += n
	return n, err
}

func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newRequestID()
		}
		ctx := withRequestID(r.Context(), requestID)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", requestID)
		recorder := &responseRecorder{ResponseWriter: w}
		defer func() {
			if recovered := recover(); recovered != nil {
				logError("http_panic", nil, logFields{"request_id": requestID, "method": r.Method, "path": r.URL.Path, "panic": sanitizeLogText(fmt.Sprint(recovered), 1000)})
				if recorder.status == 0 {
					writeJSONResponse(recorder, http.StatusInternalServerError, map[string]any{"error": "internal server error", "request_id": requestID})
				}
			}
			status := recorder.status
			if status == 0 {
				status = http.StatusOK
			}
			fields := logFields{
				"request_id":  requestID,
				"method":      r.Method,
				"path":        r.URL.Path,
				"status":      status,
				"duration_ms": time.Since(start).Milliseconds(),
				"bytes":       recorder.bytes,
			}
			if status >= 500 {
				logError("http_request", nil, fields)
			} else {
				logInfo("http_request", fields)
			}
		}()
		next.ServeHTTP(recorder, r)
	})
}

func (a *App) authMiddleware(next http.Handler) http.Handler {
	token := strings.TrimSpace(a.cfg.AuthToken)
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isBackendRoute(r.URL.Path) || isPublicBackendRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		got := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if got != token {
			writeJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized", "request_id": requestIDFromRequest(r)})
			return
		}
		next.ServeHTTP(w, r)
	})
}
