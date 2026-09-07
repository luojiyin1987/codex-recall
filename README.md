# codex-recall

Search your local Codex history by code, project, command, error, or conversation — then inspect, reopen, or resume the exact session.

`codex-recall` is intentionally read-only. It discovers local Codex rollout files and extracts the session and conversation data needed for browsing and search. It does not modify Codex's own session data.

## Quick start

Search your Codex history:

```bash
cxq search "WebRTC"
```

A result includes the Codex session ID:

```text
DATE              PROJECT       SOURCE  ROLE  SESSION                               MATCH
2026-08-08 17:54  cve-lite-cli  vscode  user  019fe0cb-9760-78b1-b545-b5e90d1dd0d7  ... WebRTC ...
```

Use the full ID or a unique prefix:

```bash
cxq open 019fe0cb
```

After finding a session:

```text
cxq search "query"
        |
        +--> cxq open SESSION    # return to the original client
        +--> cxq show SESSION    # inspect in this terminal
        +--> cxq resume SESSION  # force Codex CLI resume
```

For a large history or frequent searches, build a local index once and reuse it:

```bash
cxq index
cxq search --index "WebRTC"
```

## Installation

### Prebuilt binaries

Tagged releases publish native `cxq` binaries for Linux, Windows, and macOS on x64 and ARM64. Download the archive for your platform from [GitHub Releases](https://github.com/luojiyin1987/codex-recall/releases).

Linux and macOS:

```bash
tar -xzf cxq_0.1.0_linux_x64.tar.gz
mkdir -p ~/.local/bin
install -m 0755 cxq ~/.local/bin/cxq
```

Make sure `~/.local/bin` is on `PATH`.

On Windows, extract `cxq.exe` from the matching `.zip` archive and place it in a directory on `PATH`.

Each release includes `SHA256SUMS`.

### Go install

Go 1.23 or newer is recommended.

```bash
go install github.com/luojiyin1987/codex-recall/cmd/cxq@latest
cxq --version
```

Make sure the Go binary directory, usually `$GOPATH/bin` or `~/go/bin`, is on `PATH`.

## Commands

| Command | Purpose |
| --- | --- |
| `cxq list` | List local Codex sessions |
| `cxq search QUERY` | Search live rollout files |
| `cxq index` | Build or refresh the derived SQLite index |
| `cxq search --index QUERY` | Search the derived index |
| `cxq status` | Inspect the existing index |
| `cxq compare QUERY` | Compare live and indexed result sets |
| `cxq pack QUERY` | Build a deterministic context pack from indexed evidence |
| `cxq show SESSION` | Print a conversation in the terminal |
| `cxq open SESSION` | Open a session in its original client |
| `cxq resume SESSION` | Resume a session explicitly with Codex CLI |

Run `cxq help` for the command summary.

## Search

Search user and assistant conversation text:

```bash
cxq search "Promise"
cxq search --limit 5 "annotated tag"
cxq search --project lint-md "Promise"
cxq search --source vscode "annotated tag"
cxq search --project cve-lite-cli --source vscode "tag"
```

Live search is a case-insensitive literal match. Tool output, reasoning records, and session metadata are excluded.

Search returns at most 20 sessions by default and scans candidates from newest to oldest. Increase the limit when needed:

```bash
cxq search --limit 100 "Promise"
```

List sessions without a text query:

```bash
cxq list
cxq list --project deepseek-harness-remote
cxq list --source vscode
```

`--project` and `--source` use case-insensitive exact matching after trimming surrounding whitespace.

## Indexed search

Live search reads Codex rollout files on every query. Indexed search moves that work into an explicit refresh step so repeated searches can reuse a derived SQLite/FTS5 index.

```text
Codex rollout files
        |
        |  cxq index
        v
SQLite + FTS5 trigram
        |
        |  cxq search --index QUERY
        v
ranked search results
```

Build or refresh the index:

```bash
cxq index
```

Search it:

```bash
cxq search --index "WebRTC"
cxq search --index --project deepseek-harness-remote "WebRTC"
```

Explain why each indexed result was selected:

```bash
cxq search --index --explain "WebRTC"
```

`--explain` adds the matched message ordinal, retrieval score, and retrieval reason to the terminal table. FTS5 trigram results report `lexical:fts5`; one- and two-character literal fallback results report `lexical:substring`. Indexed JSON output already includes the same `ordinal`, `score`, and `why` fields, so its schema is unchanged.

Profile the indexed retrieval path:

```bash
cxq search --index --profile "WebRTC"
```

`--profile` writes lightweight timing and cardinality evidence to stderr, leaving the normal table or `--json` output on stdout unchanged. It reports the retrieval backend (`fts5` or `substring`), database-open time, query setup, result scanning, snippet formatting, total indexed-search time, rows scanned, and sessions returned. `RESULT_SCAN` excludes the separately reported `SNIPPET` time.

Inspect it without refreshing:

```bash
cxq status
```

`status` reports the database path, indexed session count, newest indexed session timestamp, and database size. It requires an existing index and does not create one implicitly.

The rollout files remain the source of truth. The SQLite database is derived data and can be rebuilt.

### Index behavior

Schema v3 uses FTS5 `trigram` search for queries of three or more Unicode characters. This supports substring-shaped lookups such as Chinese text, camelCase prefixes, UUID prefixes, and commit-SHA prefixes.

One- and two-character queries use a case-insensitive literal scan of the derived `messages` table because trigram MATCH cannot produce shorter tokens.

FTS results use BM25 ranking. Indexed results remain session-based, so `--limit` counts unique sessions.

Index refresh is explicit:

```text
new Codex conversations
        |
        |  cxq index
        v
updated derived index
```

`cxq search --index`, `cxq status`, `cxq compare`, and `cxq pack` do not silently refresh the database.

A clean refresh also removes stale derived sessions when their source rollouts no longer exist. If discovery or parsing reports warnings, stale deletion is skipped for that refresh to avoid destructive reconciliation from uncertain source data.

### Compare live and indexed search

Use the same query against both backends:

```bash
cxq compare "WebRTC"
cxq compare --project deepseek-harness-remote "WebRTC"
```

The summary includes:

- `OVERLAP`: sessions returned by both backends
- `LIVE_ONLY`: sessions returned only by the live scanner
- `INDEX_ONLY`: sessions returned only by the index

This is useful for inspecting retrieval differences and detecting a stale index.

### Context packs

Build a deterministic context pack from indexed search evidence:

```bash
cxq pack --project codex-recall "sqlite index"
cxq pack --limit 10 --project codex-recall "sqlite index"
```

The default limit is 5 sessions. Each evidence item includes its session ID, timestamp, project, source, role, message ordinal, snippet, retrieval score, retrieval reason, and an exact `cxq resume SESSION` command.

`pack` does not call an LLM, create a memory database, extract decisions or todos, or refresh the index implicitly.

## Use a search result

A session ID can be referenced by its full UUID or by a unique prefix.

### Inspect without launching Codex

```bash
cxq show 019fe0cb
```

`show` prints session metadata followed by user and assistant messages. Tool output, reasoning, and metadata records are omitted.

### Return to the original client

```bash
cxq open 019fe0cb
```

`open` uses the stored session source:

```text
source=vscode  --> Codex VS Code extension
source=cli     --> Codex CLI
```

Use an explicit target when needed:

```bash
cxq open --target vscode 019fe0cb
cxq open --target cli 019fe0cb
```

VS Code Insiders:

```bash
cxq open --vscode-scheme vscode-insiders 019fe0cb
```

In a VS Code WSL terminal, `open` uses the current VS Code IPC channel so the active WSL window is preserved. Other WSL terminals use the Windows URI handler.

### Force Codex CLI resume

```bash
cxq resume 019fe0cb
```

`resume` always runs the official Codex CLI, regardless of the session's original source.

When the original working directory still exists, Codex starts from that directory. Otherwise `cxq` warns and falls back to the current directory.

The `codex` executable must be available on `PATH`.

## JSON output

Machine-readable output is available for the main retrieval commands:

```bash
cxq list --json --project deepseek-harness-remote
cxq search --json "Promise"
cxq search --index --json "WebRTC"
cxq status --json
cxq compare --json "WebRTC"
cxq pack --json "sqlite index"
```

JSON output uses `schema_version: 1`. Timestamps use RFC3339Nano in UTC. Warnings remain on stderr.

Live and indexed search results share the same result shape. Index-only metadata such as `ordinal`, `score`, and `why` is `null` for live results.

## Custom paths

Codex sessions are discovered below `$CODEX_HOME/sessions` and `$CODEX_HOME/archived_sessions`, or the corresponding directories under `~/.codex`.

Use another Codex home:

```bash
cxq list --home /path/to/.codex
cxq search --home /path/to/.codex "Promise"
cxq show --home /path/to/.codex 019fe0cb
cxq open --home /path/to/.codex 019fe0cb
cxq resume --home /path/to/.codex 019fe0cb
```

Use a custom index database:

```bash
cxq index --db /path/to/index.db
cxq status --db /path/to/index.db
cxq search --index --db /path/to/index.db "WebRTC"
cxq compare --db /path/to/index.db "WebRTC"
cxq pack --db /path/to/index.db "WebRTC"
```

## Optional: ripgrep

Live `cxq search` uses [ripgrep](https://github.com/BurntSushi/ripgrep) (`rg`) as a fast candidate filter when available.

It is optional. Without `rg`, `cxq` falls back to its built-in scanner.

```bash
# Debian / Ubuntu / WSL
sudo apt-get install ripgrep

# macOS
brew install ripgrep

# Windows
winget install BurntSushi.ripgrep.MSVC
```

## Retrieval evaluation

The repository includes an anonymized retrieval corpus covering natural-language phrases, Chinese text, camelCase and snake_case identifiers, file paths, runtime errors, UUID/SHA lookups, method calls, CLI flags, punctuation, and ranking cases.

Run it with:

```bash
go test ./internal/indexer -run TestRetrievalEvaluationCorpus -v
```

The evaluation measures live and indexed retrieval at Hit@5 and reports per-query rank plus aggregate MRR. The fixture is synthetic and anonymized; it captures realistic query shapes without committing private Codex history.

### Trigram experiment

The reproducible A/B harness compares the historical `unicode61` tokenizer with `trigram`:

```bash
go test ./internal/indexer -run TestTrigramRetrievalExperiment -v
```

It reports Hit@5, MRR, database size, build time, search time, and categorized misses.

Production indexed search now uses trigram. The experiment remains as evidence for that design decision.

## Benchmarks

Benchmarks are opt-in:

```bash
go test ./internal/index ./internal/indexer ./internal/codex -run '^$' -bench . -benchmem
```

The benchmark datasets cover 100, 1,000, and 10,000 sessions. Index benchmarks report database size alongside Go's standard timing and allocation metrics. The live-search benchmark disables the optional ripgrep fast path so results are comparable across machines.

Tokenizer-specific benchmarks:

```bash
go test ./internal/indexer -run '^$' -bench 'BenchmarkRetrievalTokenizer' -benchmem
```

## Build

```bash
go build -o cxq ./cmd/cxq
```

During development, `go run` avoids accidentally executing a stale binary:

```bash
go run ./cmd/cxq search "Promise"
```

## Platform notes

The supported Go toolchain starts at Go 1.23.

Compatibility probing on 2026-09-04 found that Go 1.22.12 works on Linux x64 and macOS x64 but fails on current macOS ARM64 because generated binaries can abort with a missing `LC_UUID` load command. Go 1.23.12 passed tests, native build, and SQLite index/search smoke tests on Linux x64, macOS x64, and macOS ARM64. Go 1.24.13 also passed the probes.

Go 1.22 remains a non-blocking compatibility probe; Go 1.23 and 1.24 are blocking supported-toolchain checks.

## Design principles

- Treat Codex session files as read-only input.
- Treat the SQLite database as a rebuildable derived index.
- Tolerate unknown JSONL event types.
- Search and show conversation text, not tool noise or reasoning data.
- Delegate CLI session resumption to the official Codex CLI.
- Delegate VS Code session display to the Codex extension.
- Keep live search and indexed search independently testable.
- Prefer a small, cross-platform CLI with minimal dependencies.
- Treat Ctrl+C as cooperative cancellation for indexing and search work.
