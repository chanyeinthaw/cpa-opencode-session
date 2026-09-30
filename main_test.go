package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

func TestResolveSessionIDPriority(t *testing.T) {
	headers := http.Header{
		"Thread-Id":             {"thread-123"},
		"Session-Id":            {"session-123"},
		"X-Codex-Turn-Metadata": {`{"thread_id":"metadata-thread","session_id":"metadata-session"}`},
	}
	if got := resolveSessionID(headers, []byte(`{"prompt_cache_key":"cache-123"}`)); got != "thread-123" {
		t.Fatalf("resolveSessionID() = %q, want thread-123", got)
	}
}

func TestResolveSessionIDFallbacks(t *testing.T) {
	tests := []struct {
		name    string
		headers http.Header
		body    []byte
		want    string
	}{
		{
			name:    "Codex metadata thread",
			headers: http.Header{"X-Codex-Turn-Metadata": {`{"thread_id":"metadata-thread"}`}},
			want:    "metadata-thread",
		},
		{
			name:    "session header",
			headers: http.Header{"Session-Id": {"session-123"}},
			want:    "session-123",
		},
		{
			name:    "Codex metadata session",
			headers: http.Header{"X-Codex-Turn-Metadata": {`{"session_id":"metadata-session"}`}},
			want:    "metadata-session",
		},
		{
			name:    "Claude session header",
			headers: http.Header{"X-Claude-Code-Session-Id": {"claude-session"}},
			want:    "claude-session",
		},
		{
			name: "Claude user metadata",
			body: []byte(`{"metadata":{"user_id":"{\"device_id\":\"device-123\",\"account_uuid\":\"account-123\",\"session_id\":\"claude-session\"}"}}`),
			want: "claude-session",
		},
		{
			name:    "Claude header precedes user metadata and cache key",
			headers: http.Header{"X-Claude-Code-Session-Id": {"header-session"}},
			body:    []byte(`{"metadata":{"user_id":"{\"session_id\":\"body-session\"}"},"prompt_cache_key":"cache-123"}`),
			want:    "header-session",
		},
		{
			name: "Claude metadata precedes cache key",
			body: []byte(`{"metadata":{"user_id":"{\"session_id\":\"body-session\"}"},"prompt_cache_key":"cache-123"}`),
			want: "body-session",
		},
		{
			name: "Malformed Claude metadata falls back to cache key",
			body: []byte(`{"metadata":{"user_id":"not JSON"},"prompt_cache_key":"cache-123"}`),
			want: "cache-123",
		},
		{
			name: "Claude metadata without a session",
			body: []byte(`{"metadata":{"user_id":"{\"device_id\":\"device-123\"}"}}`),
		},
		{
			name: "prompt cache key",
			body: []byte(`{"prompt_cache_key":"cache-123"}`),
			want: "cache-123",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveSessionID(test.headers, test.body); got != test.want {
				t.Fatalf("resolveSessionID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInterceptAfterAuthAddsSessionForOCGModel(t *testing.T) {
	tests := []struct {
		name    string
		request pluginapi.RequestInterceptRequest
	}{
		{
			name: "requested model",
			request: pluginapi.RequestInterceptRequest{
				RequestedModel: "ocg/muse-spark-1.3-contributor",
				Headers:        http.Header{"Thread-Id": {"thread-123"}},
			},
		},
		{
			name: "requested model metadata",
			request: pluginapi.RequestInterceptRequest{
				RequestedModel: "glm-5.3-flash",
				Metadata:       map[string]any{"requested_model": "ocg/glm-5.3-flash"},
				Headers:        http.Header{"Thread-Id": {"thread-123"}},
			},
		},
		{
			name: "request body model",
			request: pluginapi.RequestInterceptRequest{
				RequestedModel: "glm-5.3-flash",
				Body:           []byte(`{"model":"ocg/glm-5.3-flash"}`),
				Headers:        http.Header{"Thread-Id": {"thread-123"}},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := interceptForTest(t, test.request)
			if got := response.Headers.Get(opencodeSession); got != "thread-123" {
				t.Fatalf("%s = %q, want thread-123", opencodeSession, got)
			}
		})
	}
}

func TestInterceptAfterAuthPreservesExplicitSession(t *testing.T) {
	response := interceptForTest(t, pluginapi.RequestInterceptRequest{
		RequestedModel: "ocg/glm-5.3-flash",
		Headers: http.Header{
			opencodeSession: {"explicit-session"},
			"Thread-Id":     {"thread-123"},
		},
	})
	if response.Headers.Get(opencodeSession) != "" {
		t.Fatalf("interceptor unexpectedly replaced explicit session: %#v", response.Headers)
	}
}

func TestInterceptAfterAuthIgnoresOtherModels(t *testing.T) {
	response := interceptForTest(t, pluginapi.RequestInterceptRequest{
		RequestedModel: "gpt-5.6-sol",
		Headers:        http.Header{"Thread-Id": {"thread-123"}},
	})
	if response.Headers.Get(opencodeSession) != "" {
		t.Fatalf("interceptor modified non-OCG request: %#v", response.Headers)
	}
}

func TestDifferentThreadsGetDifferentSessions(t *testing.T) {
	root := interceptForTest(t, pluginapi.RequestInterceptRequest{
		RequestedModel: "ocg/muse-spark-1.3-contributor",
		Headers:        http.Header{"Thread-Id": {"root-thread"}},
	})
	child := interceptForTest(t, pluginapi.RequestInterceptRequest{
		RequestedModel: "ocg/muse-spark-1.3-contributor",
		Headers:        http.Header{"Thread-Id": {"child-thread"}},
	})
	if root.Headers.Get(opencodeSession) == child.Headers.Get(opencodeSession) {
		t.Fatal("root and child requests received the same OpenCode session")
	}
}

func TestDifferentClaudeSessionsGetDifferentOpenCodeSessions(t *testing.T) {
	for _, sessionID := range []string{"claude-session-one", "claude-session-two"} {
		response := interceptForTest(t, pluginapi.RequestInterceptRequest{
			RequestedModel: "ocg/glm-5.3-flash",
			Body:           []byte(`{"metadata":{"user_id":"{\"session_id\":\"` + sessionID + `\"}"}}`),
		})
		if got := response.Headers.Get(opencodeSession); got != sessionID {
			t.Fatalf("%s = %q, want %q", opencodeSession, got, sessionID)
		}
	}
}

func interceptForTest(t *testing.T, req pluginapi.RequestInterceptRequest) pluginapi.RequestInterceptResponse {
	t.Helper()
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	enveloped, err := interceptRequest(raw)
	if err != nil {
		t.Fatalf("interceptRequest(): %v", err)
	}
	var env envelope
	if err := json.Unmarshal(enveloped, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	var response pluginapi.RequestInterceptResponse
	if err := json.Unmarshal(env.Result, &response); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return response
}
