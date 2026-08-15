package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIPathCompatibility(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		target       string
		headers      map[string]string
		expectedPath string
		expectedURI  string
	}{
		{
			name:         "adds prefix to chat completions",
			method:       http.MethodPost,
			target:       "/chat/completions?trace=true",
			expectedPath: "/v1/chat/completions",
			expectedURI:  "/v1/chat/completions?trace=true",
		},
		{
			name:         "preserves existing versioned path",
			method:       http.MethodPost,
			target:       "/v1/chat/completions",
			expectedPath: "/v1/chat/completions",
			expectedURI:  "/v1/chat/completions",
		},
		{
			name:         "supports authenticated model listing",
			method:       http.MethodGet,
			target:       "/models",
			headers:      map[string]string{"Authorization": "Bearer sk-test"},
			expectedPath: "/v1/models",
			expectedURI:  "/v1/models",
		},
		{
			name:         "leaves unauthenticated frontend models path alone",
			method:       http.MethodGet,
			target:       "/models",
			expectedPath: "/models",
			expectedURI:  "/models",
		},
		{
			name:         "supports realtime websocket authentication",
			method:       http.MethodGet,
			target:       "/realtime",
			headers:      map[string]string{"Sec-WebSocket-Protocol": "realtime, openai-insecure-api-key.sk-test"},
			expectedPath: "/v1/realtime",
			expectedURI:  "/v1/realtime",
		},
		{
			name:         "supports video content fetch",
			method:       http.MethodGet,
			target:       "/videos/task-1/content",
			headers:      map[string]string{"Authorization": "Bearer sk-test"},
			expectedPath: "/v1/videos/task-1/content",
			expectedURI:  "/v1/videos/task-1/content",
		},
		{
			name:         "supports video remix",
			method:       http.MethodPost,
			target:       "/videos/video-1/remix",
			expectedPath: "/v1/videos/video-1/remix",
			expectedURI:  "/v1/videos/video-1/remix",
		},
		{
			name:   "supports cors preflight for model listing",
			method: http.MethodOptions,
			target: "/models",
			headers: map[string]string{
				"Access-Control-Request-Method": "GET",
				"Origin":                        "https://example.com",
			},
			expectedPath: "/v1/models",
			expectedURI:  "/v1/models",
		},
		{
			name:   "supports cors preflight for chat completions",
			method: http.MethodOptions,
			target: "/chat/completions",
			headers: map[string]string{
				"Access-Control-Request-Method": "POST",
				"Origin":                        "https://example.com",
			},
			expectedPath: "/v1/chat/completions",
			expectedURI:  "/v1/chat/completions",
		},
		{
			name:         "leaves claude protocol path alone",
			method:       http.MethodPost,
			target:       "/messages",
			expectedPath: "/messages",
			expectedURI:  "/messages",
		},
		{
			name:         "leaves gemini protocol path alone",
			method:       http.MethodPost,
			target:       "/v1beta/models/gemini:generateContent",
			expectedPath: "/v1beta/models/gemini:generateContent",
			expectedURI:  "/v1beta/models/gemini:generateContent",
		},
		{
			name:         "leaves management API path alone",
			method:       http.MethodGet,
			target:       "/api/models",
			headers:      map[string]string{"Authorization": "Bearer sk-test"},
			expectedPath: "/api/models",
			expectedURI:  "/api/models",
		},
		{
			name:         "does not rewrite unsupported nested path",
			method:       http.MethodPost,
			target:       "/chat/completions/batch",
			expectedPath: "/chat/completions/batch",
			expectedURI:  "/chat/completions/batch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.target, nil)
			for key, value := range tt.headers {
				request.Header.Set(key, value)
			}

			called := false
			handler := OpenAIPathCompatibility(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				called = true
				assert.Equal(t, tt.expectedPath, r.URL.Path)
				assert.Equal(t, tt.expectedURI, r.RequestURI)
			}))

			handler.ServeHTTP(httptest.NewRecorder(), request)
			require.True(t, called)
		})
	}
}

func TestOpenAIPathCompatibilityRunsBeforeGinRouteMatching(t *testing.T) {
	engine := http.NewServeMux()
	engine.HandleFunc("POST /v1/responses", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	})

	request := httptest.NewRequest(http.MethodPost, "/responses", nil)
	response := httptest.NewRecorder()
	OpenAIPathCompatibility(engine).ServeHTTP(response, request)

	assert.Equal(t, http.StatusAccepted, response.Code)
}
