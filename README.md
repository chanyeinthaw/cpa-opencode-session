# CPA OpenCode session

A CLIProxyAPI request-interceptor plugin that derives `X-OpenCode-Session` from request-scoped Codex metadata for `ocg/*` models.

Resolution order:

1. Existing `X-OpenCode-Session` is preserved.
2. `Thread-Id`
3. `X-Codex-Turn-Metadata.thread_id`
4. `Session-Id`
5. `X-Codex-Turn-Metadata.session_id`
6. `prompt_cache_key`

Each Codex subagent has its own thread ID, so root and child agents receive separate OpenCode sessions.

## Build

```bash
make check
```

Install `dist/opencode-session-v0.1.6.so` into CPA's configured plugin directory and enable it. For OpenAI-compatible providers, CPA must also be told to forward the plugin-produced header:

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
