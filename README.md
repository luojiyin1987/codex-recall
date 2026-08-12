# codex-recall

Search, find, and resume your local Codex conversations.

`codex-recall` is intentionally read-only: it discovers local Codex rollout files and extracts the session and conversation data needed for browsing and search. It does not modify Codex's own session data.

## Commands

List local sessions:

```bash
cxq list
```

Search conversation text:

```bash
cxq search "Promise"
cxq search --limit 5 "annotated tag"
```

Search is a case-insensitive literal match over user and assistant conversation text. Tool output, reasoning records, and session metadata are excluded. The first matching message from each session is shown with a compact snippet.

```text
DATE              PROJECT  ROLE  SESSION   MATCH
2026-08-09 10:01  lint-md  user  019abc... ... why does Promise need a controlled pause? ...
```

Show a conversation by full session ID or a unique prefix:

```bash
cxq show 019fe0cb
```

`show` prints session metadata followed by user and assistant messages. Tool output, reasoning, and metadata records are omitted.

Resume the same session with the official Codex CLI:

```bash
cxq resume 019fe0cb
```

`resume` resolves the prefix to the full local session ID and then runs `codex resume <session-id>` with the terminal attached. The `codex` executable must be available on `PATH`.

The CLI discovers JSONL rollout files below `$CODEX_HOME/sessions` and `$CODEX_HOME/archived_sessions`, or the corresponding directories under `~/.codex`.

An alternate Codex home can be supplied explicitly:

```bash
cxq list --home /path/to/.codex
cxq search --home /path/to/.codex "Promise"
cxq show --home /path/to/.codex 019fe0cb
cxq resume --home /path/to/.codex 019fe0cb
```

## Build

```bash
go build -o cxq ./cmd/cxq
```

During development, `go run` avoids accidentally executing a stale binary:

```bash
go run ./cmd/cxq search "Promise"
```

## Design principles

- Treat Codex session files as read-only input.
- Tolerate unknown JSONL event types.
- Search and show conversation text, not tool noise or reasoning data.
- Delegate session resumption to the official Codex CLI.
- Keep discovery, parsing, and search independent from future indexing layers.
- Prefer a small, cross-platform CLI with minimal dependencies.
