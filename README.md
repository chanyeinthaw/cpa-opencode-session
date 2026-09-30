# CPA OpenCode session

A CLIProxyAPI request-interceptor plugin that derives `X-OpenCode-Session` from request-scoped Codex or Claude Code session metadata for `ocg/*` models.

Resolution order:

1. Existing `X-OpenCode-Session` is preserved.
2. `Thread-Id`
3. `X-Codex-Turn-Metadata.thread_id`
4. `Session-Id`
5. `X-Codex-Turn-Metadata.session_id`
6. `X-Claude-Code-Session-Id`
7. `metadata.user_id`, a JSON string containing `session_id` in Claude Code requests
8. `prompt_cache_key`

Each Codex subagent has its own thread ID, so root and child agents receive separate OpenCode sessions.

Claude Code requests use their session ID without the device ID or account UUID. Explicit OpenCode headers still take precedence, so omit hardcoded client headers to use automatic session routing. Requests with no recognized session metadata remain unchanged.

## Build

```bash
make check
```

Install `dist/opencode-session-v0.1.7.so` into CPA's configured plugin directory and enable it. For OpenAI-compatible providers, CPA must also be told to forward the plugin-produced header:

```yaml
plugins:
  enabled: true
  configs:
    opencode-session:
      enabled: true
      priority: 100

openai-compatibility:
  - name: Opencode Go
    headers:
      X-OpenCode-Session: "$X-OpenCode-Session"
```
