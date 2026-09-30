package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unsafe"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginabi"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/pluginapi"
)

const (
	pluginName      = "opencode-session"
	pluginVersion   = "0.1.7"
	opencodePrefix  = "ocg/"
	opencodeSession = "X-OpenCode-Session"
)

type envelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *envelopeError  `json:"error,omitempty"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type registration struct {
	SchemaVersion uint32                 `json:"schema_version"`
	Metadata      pluginapi.Metadata     `json:"metadata"`
	Capabilities  registrationCapability `json:"capabilities"`
}

type registrationCapability struct {
	RequestInterceptor bool `json:"request_interceptor"`
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = C.uint32_t(pluginabi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}

	var requestBytes []byte
	if request != nil && requestLen > 0 {
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, err := handleMethod(C.GoString(method), requestBytes)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func handleMethod(method string, request []byte) ([]byte, error) {
	switch method {
	case pluginabi.MethodPluginRegister, pluginabi.MethodPluginReconfigure:
		return okEnvelope(registration{
			SchemaVersion: pluginabi.SchemaVersion,
			Metadata: pluginapi.Metadata{
				Name:             pluginName,
				Version:          pluginVersion,
				Author:           "Chan Nyein Thaw",
				GitHubRepository: "https://github.com/chanyeinthaw/cpa-opencode-session",
			},
			Capabilities: registrationCapability{RequestInterceptor: true},
		})
	case pluginabi.MethodRequestInterceptBefore, pluginabi.MethodRequestInterceptAfter:
		return interceptRequest(request)
	default:
		return errorEnvelope("unknown_method", "unknown method: "+method), nil
	}
}

func interceptRequest(raw []byte) ([]byte, error) {
	var req pluginapi.RequestInterceptRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	if !isOpenCodeRequest(req) || headerValue(req.Headers, opencodeSession) != "" {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}

	sessionID := resolveSessionID(req.Headers, req.Body)
	if sessionID == "" {
		return okEnvelope(pluginapi.RequestInterceptResponse{})
	}

	headers := make(http.Header)
	headers.Set(opencodeSession, sessionID)
	return okEnvelope(pluginapi.RequestInterceptResponse{Headers: headers})
}

func isOpenCodeRequest(req pluginapi.RequestInterceptRequest) bool {
	for _, model := range []string{
		req.RequestedModel,
		metadataString(req.Metadata, "requested_model"),
		requestBodyModel(req.Body),
	} {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), opencodePrefix) {
			return true
		}
	}
	return false
}

func requestBodyModel(body []byte) string {
	var payload struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.Model)
}

func metadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	switch value := metadata[key].(type) {
	case string:
		return strings.TrimSpace(value)
	case []byte:
		return strings.TrimSpace(string(value))
	default:
		return ""
	}
}

func resolveSessionID(headers http.Header, body []byte) string {
	if value := headerValue(headers, "Thread-Id"); value != "" {
		return value
	}
	if value := codexMetadataValue(headers, "thread_id"); value != "" {
		return value
	}
	if value := headerValue(headers, "Session-Id"); value != "" {
		return value
	}
	if value := codexMetadataValue(headers, "session_id"); value != "" {
		return value
	}
	if value := headerValue(headers, "X-Claude-Code-Session-Id"); value != "" {
		return value
	}

	var payload struct {
		PromptCacheKey string `json:"prompt_cache_key"`
		Metadata       struct {
			UserID string `json:"user_id"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &payload) == nil {
		// Claude Code encodes its session metadata as JSON inside user_id.
		var userMetadata struct {
			SessionID string `json:"session_id"`
		}
		if json.Unmarshal([]byte(payload.Metadata.UserID), &userMetadata) == nil {
			if value := strings.TrimSpace(userMetadata.SessionID); value != "" {
				return value
			}
		}
		return strings.TrimSpace(payload.PromptCacheKey)
	}
	return ""
}

func headerValue(headers http.Header, name string) string {
	for key, values := range headers {
		if strings.EqualFold(key, name) && len(values) > 0 {
			return strings.TrimSpace(values[0])
		}
	}
	return ""
}

func codexMetadataValue(headers http.Header, name string) string {
	raw := headerValue(headers, "X-Codex-Turn-Metadata")
	if raw == "" {
		return ""
	}
	var metadata map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &metadata) != nil {
		return ""
	}
	value, ok := metadata[name]
	if !ok {
		return ""
	}
	var decoded string
	if json.Unmarshal(value, &decoded) != nil {
		return ""
	}
	return strings.TrimSpace(decoded)
}

func okEnvelope(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{OK: true, Result: raw})
}

func errorEnvelope(code, message string) []byte {
	raw, err := json.Marshal(envelope{OK: false, Error: &envelopeError{Code: code, Message: message}})
	if err != nil {
		return []byte(fmt.Sprintf(`{"ok":false,"error":{"code":"%s","message":"encode error"}}`, code))
	}
	return raw
}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
