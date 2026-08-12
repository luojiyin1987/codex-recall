# codex-recall

Search, find, and resume your local Codex conversations.

`codex-recall` is intentionally read-only: it discovers local Codex rollout files and extracts the small amount of session metadata needed for browsing. It does not modify Codex's own session data.

## Current scope

PR #1 introduces the first command:

```bash
cxq list
```

It discovers JSONL rollout files below `$CODEX_HOME/sessions` and `$CODEX_HOME/archived_sessions` (or the corresponding directories under `~/.codex`) and lists the session timestamp, project, source, and session ID.

```text
DATE              PROJECT       SOURCE  SESSION
2026-08-12 10:59  codex-recall  vscode  019abc...
```

An alternate Codex home can be supplied explicitly:

```bash
cxq list --home /path/to/.codex
```

## Build

```bash
go build -o cxq ./cmd/cxq
```

## Design principles

- Treat Codex session files as read-only input.
- Tolerate unknown JSONL event types.
- Keep discovery and parsing independent from future indexing/search layers.
- Prefer a small, cross-platform CLI with minimal dependencies.
