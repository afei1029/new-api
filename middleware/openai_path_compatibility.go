package middleware

import (
	"net/http"
	"strings"
)

var openAICompatiblePostPaths = map[string]struct{}{
	"/alpha/search":         {},
	"/audio/speech":         {},
	"/audio/transcriptions": {},
	"/audio/translations":   {},
	"/chat/completions":     {},
	"/completions":          {},
	"/edits":                {},
	"/embeddings":           {},
	"/images/edits":         {},
	"/images/generations":   {},
	"/moderations":          {},
	"/rerank":               {},
	"/responses":            {},
	"/responses/compact":    {},
	"/video/generations":    {},
	"/videos":               {},
}

// OpenAIPathCompatibility accepts supported OpenAI endpoints both with and
// without the conventional /v1 prefix. It must wrap Gin so normalization runs
// before Gin performs route matching.
func OpenAIPathCompatibility(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isUnversionedOpenAIRequest(r) {
			r.URL.Path = "/v1" + r.URL.Path
			r.URL.RawPath = ""
			r.RequestURI = "/v1" + r.RequestURI
		}
		next.ServeHTTP(w, r)
	})
}

func isUnversionedOpenAIRequest(r *http.Request) bool {
	path := r.URL.Path
	if path == "" || path == "/" || strings.HasPrefix(path, "/v1/") || strings.HasPrefix(path, "/v1beta/") {
		return false
	}

	method := r.Method
	isPreflight := method == http.MethodOptions
	if isPreflight {
		method = strings.ToUpper(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")))
	}

	switch method {
	case http.MethodPost:
		if _, ok := openAICompatiblePostPaths[path]; ok {
			return true
		}
		return matchesResourceAction(path, "/videos/", "/remix")
	case http.MethodGet:
		if !isPreflight && !hasBearerAuthorization(r.Header.Get("Authorization")) {
			return path == "/realtime" && r.Header.Get("Sec-WebSocket-Protocol") != ""
		}
		return path == "/models" ||
			path == "/realtime" ||
			hasSinglePathSegmentAfter(path, "/models/") ||
			hasSinglePathSegmentAfter(path, "/video/generations/") ||
			hasSinglePathSegmentAfter(path, "/videos/") ||
			matchesResourceAction(path, "/videos/", "/content")
	}

	return false
}

func hasBearerAuthorization(value string) bool {
	scheme, token, ok := strings.Cut(strings.TrimSpace(value), " ")
	return ok && strings.EqualFold(scheme, "Bearer") && strings.TrimSpace(token) != ""
}

func hasSinglePathSegmentAfter(path string, prefix string) bool {
	value := strings.TrimPrefix(path, prefix)
	return value != path && value != "" && !strings.Contains(value, "/")
}

func matchesResourceAction(path string, prefix string, action string) bool {
	value := strings.TrimPrefix(path, prefix)
	if value == path || !strings.HasSuffix(value, action) {
		return false
	}
	resourceID := strings.TrimSuffix(value, action)
	return resourceID != "" && !strings.Contains(resourceID, "/")
}
