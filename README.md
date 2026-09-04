# codex-recall

Search, find, and open your local Codex conversations.

`codex-recall` is intentionally read-only: it discovers local Codex rollout files and extracts the session and conversation data needed for browsing and search. It does not modify Codex's own session data.

## Installation

### Prebuilt binaries

Tagged releases publish native `cxq` binaries for Linux, Windows, and macOS on both x64 and ARM64. Download the archive for your platform from [GitHub Releases](https://github.com/luojiyin1987/codex-recall/releases).

Linux and macOS archives contain a single `cxq` binary. For example:

```bash
tar -xzf cxq_0.1.0_linux_x64.tar.gz
mkdir -p ~/.local/bin
install -m 0755 cxq ~/.local/bin/cxq
```

Make sure `~/.local/bin` is on `PATH`.

On Windows, extract `cxq.exe` from the matching `.zip` archive and place it in a directory on `PATH`.

Each release also includes `SHA256SUMS` for verifying downloaded archives.

### Go install

The module requires Go 1.23 or newer. Compatibility probing on 2026-09-04 showed that Go 1.22.12 works on Linux x64 and macOS x64 but fails on current macOS ARM64 because generated test and application binaries abort with a missing `LC_UUID` load command. Go 1.23.12 passes tests, native build, and an SQLite index/search smoke test on Linux x64, macOS x64, and macOS ARM64. Go 1.24.13 also passes all probes and emits `LC_UUID` on macOS. Go 1.22 remains a non-blocking compatibility probe; Go 1.23 and 1.24 are blocking supported-toolchain checks.

If Go is already installed:

```bash
go install github.com/luojiyin1987/codex-recall/cmd/cxq@latest
```

Make sure the Go binary directory (usually `$GOPATH/bin` or `~/go/bin`) is on `PATH`, then verify:

```bash
cxq --version
```

## Commands

List local sessions:

```bash
cxq list
cxq list --project deepseek-harness-remote
cxq list --source vscode
cxq list --project deepseek-harness-remote --source vscode
cxq list --json --project deepseek-harness-remote
```

`list` can filter sessions without requiring a conversation-text query. `--project` and `--source` use case-insensitive exact matching after trimming surrounding whitespace. When both are supplied, both conditions must match. Machine-readable list output is available with `--json` and uses the same `schema_version: 1` contract.

Search conversation text:

```bash
cxq search "Promise"
cxq search --limit 5 "annotated tag"
cxq search --project lint-md "Promise"
cxq search --source vscode "annotated tag"
cxq search --project cve-lite-cli --source vscode "tag"
cxq search --json "Promise"
```

Search is a case-insensitive literal match over user and assistant conversation text. Tool output, reasoning records, and session metadata are excluded. The first matching message from each session is shown with a compact snippet.

Search returns at most 20 sessions by default. This command is equivalent to `cxq search --limit 20 "Promise"`.

Candidates are scanned from newest to oldest. The search stops when it reaches the limit. Older matching sessions are not scanned or displayed.

Set a larger limit when you need more results:

```bash
cxq search --limit 100 "Promise"
cxq search --limit 10000 "Promise"
```

The limit is a maximum result count. A large limit scans all candidates when fewer matching sessions exist.

### Indexed search and backend comparison

Build or refresh the derived SQLite/FTS5 index explicitly:

```bash
cxq index
```

A clean refresh also reconciles stale derived sessions: if a rollout no longer exists, its session and searchable messages are removed from the SQLite index. When catalog discovery or parsing reports warnings, stale deletion is skipped for that refresh so uncertain source files cannot cause destructive reconciliation.

Inspect the existing index without refreshing it:

```bash
cxq status
```

`status` reports the database path, indexed session count, newest indexed session timestamp, and database size in bytes. It requires an existing index and will not create one implicitly. It intentionally does not claim a last-refresh timestamp or staleness state because those are not yet tracked as explicit index metadata.

Machine-readable status output is available with:

```bash
cxq status --json
```

Indexed search is opt-in. The default `cxq search` command continues to scan the live Codex rollout files. Indexed results are also session-based: when several messages in one session match, only the highest-ranked FTS5 message represents that session, and `--limit` counts unique sessions.

```bash
cxq search --index "WebRTC"
cxq search --index --project deepseek-harness-remote "WebRTC"
cxq search --index --json "WebRTC"
```

When `--json` is used with `search`, stdout contains one JSON document with `schema_version: 1`, the selected backend, the query, and the result array. Live and indexed results share the same fields. Index-only metadata (`ordinal`, `score`, and `why`) is `null` for live results. Timestamps use RFC3339Nano in UTC. Warnings remain on stderr.

Use `compare` to run the live scanner and indexed FTS5 search with the same query and inspect where their returned session sets differ:

```bash
cxq compare "WebRTC"
cxq compare --project deepseek-harness-remote "WebRTC"
cxq compare --json "WebRTC"
```

The comparison summary reports:

- `OVERLAP`: sessions returned by both backends
- `LIVE_ONLY`: sessions returned only by the live rollout scanner
- `INDEX_ONLY`: sessions returned only by the FTS5 index
- `LIVE_RESULTS` / `INDEX_RESULTS`: top-N result counts; both limits are session-based
- `LIVE_SESSIONS` / `INDEX_SESSIONS`: unique session counts (normally equal to the corresponding result count)

Each comparison row also shows a representative live and/or indexed snippet, so the difference can be inspected directly.

With `compare --json`, stdout contains the same summary counts plus structured `entries`. Each entry keeps separate `live` and `indexed` evidence using the search JSON result shape from `schema_version: 1`; the missing side is `null`.

`compare` does not refresh the index. Run `cxq index` again when you want the indexed side to include newer rollout changes. This is intentional: stale-index differences remain visible instead of being hidden by an automatic refresh.

### Context packs

Build a compact, deterministic context pack from the existing indexed search evidence:

```bash
cxq pack --project codex-recall "sqlite index"
cxq pack --limit 10 --project codex-recall "sqlite index"
cxq pack --json --project codex-recall "sqlite index"
```

`pack` is indexed-only and never refreshes the database implicitly. The default limit is 5 sessions. Each evidence item preserves its session ID, timestamp, project, source, role, message ordinal, normalized snippet, FTS score, retrieval reason, and an exact `cxq resume SESSION` command.

The first version is intentionally deterministic retrieval packaging only. It does not extract decisions or todos, create a memory database, call an LLM, use embeddings, estimate model tokens, or resume Codex automatically. Run `cxq index` explicitly when the derived index needs refreshing.

JSON output uses `schema_version: 1` and exposes the ordered evidence array as `evidence`, making the pack directly consumable by scripts and later Agent/Skill integrations.

Use a custom index database when needed:

```bash
cxq index --db /path/to/index.db
cxq status --db /path/to/index.db
cxq status --json --db /path/to/index.db
cxq search --index --db /path/to/index.db "WebRTC"
cxq compare --db /path/to/index.db "WebRTC"
cxq pack --db /path/to/index.db "WebRTC"
```

`cxq search` also accepts `--project` and `--source` as case-insensitive exact-match filters over the displayed `PROJECT` and `SOURCE` values. When both are supplied, both conditions must match. Unlike `list`, `search` always requires exactly one non-blank `QUERY`.

```text
DATE              PROJECT       SOURCE  ROLE  SESSION                               MATCH
2026-08-08 17:54  cve-lite-cli  vscode  user  019fe0cb-9760-78b1-b545-b5e90d1dd0d7  ... annotated tag ...
```

### Use a search result

The `SESSION` column is the Codex conversation ID. Copy the full ID, or just a unique prefix, and pass it to another `cxq` command.

For example, the result above can be referenced as either:

```text
019fe0cb-9760-78b1-b545-b5e90d1dd0d7
```

or, when that prefix is unique:

```text
019fe0cb
```

The most common next step is `open`, which uses the `SOURCE` value to choose the right client:

```bash
cxq open 019fe0cb
```

For a `vscode` session, `open` opens the conversation in the Codex VS Code extension. For a `cli` session, it resumes the conversation with the Codex CLI. Other sources require `--target vscode` or `--target cli`.

You can also inspect the conversation without opening Codex:

```bash
cxq show 019fe0cb
```

Or explicitly resume it with the Codex CLI:

```bash
cxq resume 019fe0cb
```

In short:

```text
cxq search "query"
        |
        +--> cxq open SESSION    # open in the source client
        +--> cxq show SESSION    # inspect in the terminal
        +--> cxq resume SESSION  # force Codex CLI resume
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

`resume` resolves the prefix to the full local session ID and then runs `codex resume <session-id>` with the terminal attached. When the session's original working directory still exists, Codex is started from that directory; otherwise `cxq` warns and falls back to the current directory. The `codex` executable must be available on `PATH`.

Open a session in its source client:

```bash
cxq open 019fe0cb
```

`open` uses the stored session source. It opens `vscode` sessions in the Codex extension. It resumes `cli` sessions with the Codex CLI.

Use an explicit target when the stored source is not `vscode` or `cli`:

```bash
cxq open --target vscode 019fe0cb
cxq open --target cli 019fe0cb
```

VS Code Insiders uses a different URI scheme:

```bash
cxq open --vscode-scheme vscode-insiders 019fe0cb
```

The VS Code route depends on the current Codex extension URI handler. A future extension release can change this route.

In a VS Code WSL terminal, `open` sends the route through the current VS Code IPC channel. This keeps the active WSL window. Other WSL terminals use the Windows URI handler.

The CLI discovers JSONL rollout files below `$CODEX_HOME/sessions` and `$CODEX_HOME/archived_sessions`, or the corresponding directories under `~/.codex`.

An alternate Codex home can be supplied explicitly:

```bash
cxq list --home /path/to/.codex
cxq list --home /path/to/.codex --project deepseek-harness-remote
cxq search --home /path/to/.codex "Promise"
cxq show --home /path/to/.codex 019fe0cb
cxq resume --home /path/to/.codex 019fe0cb
cxq open --home /path/to/.codex 019fe0cb
```

`--home` controls where `cxq` looks up the session. For the VS Code target, it does not change the Codex home used by the running extension.

## Recommended: ripgrep

`cxq search` uses [`ripgrep`](https://github.com/BurntSushi/ripgrep) (`rg`) as a fast candidate filter when it is available. `rg` is optional: if it is not installed, `cxq` falls back to its built-in scanner, but searches over a large Codex history can be noticeably slower.

Install `ripgrep` with your platform package manager:

```bash
# Debian / Ubuntu / WSL
sudo apt-get install ripgrep

# macOS (Homebrew)
brew install ripgrep

# Windows (winget)
winget install BurntSushi.ripgrep.MSVC
```

Verify the installation:

```bash
rg --version
```

## Retrieval evaluation

The repository includes an anonymized retrieval evaluation corpus based on recurring real-world query shapes from Codex work: natural-language phrases, Chinese text, camelCase and snake_case identifiers, file paths, runtime errors, UUID/SHA lookups, method calls, CLI flags, punctuation, and a focused-vs-noisy ranking case.

Run it with:

```bash
go test ./internal/indexer -run TestRetrievalEvaluationCorpus -v
```

The evaluation builds a real derived SQLite/FTS5 index, runs both the built-in live scanner and indexed search at Hit@5, and logs per-query rank plus aggregate MRR. The corpus stores relevance ground truth and baseline floors rather than hard-coding known backend misses, so retrieval improvements can raise the score without rewriting expected results. A change only fails the test when a backend drops below the recorded baseline.

The fixture is synthetic and anonymized; it captures realistic query shapes without committing private Codex conversation history.

### Trigram experiment

The repository also includes a test-only tokenizer experiment that builds two standalone FTS5 databases from the same evaluation corpus: one with `unicode61` and one with `trigram`.

Run the comparison with:

```bash
go test ./internal/indexer -run TestTrigramRetrievalExperiment -v
```

The report includes Hit@5, MRR, database bytes, index build time, total search time, average search time, and categorized misses for each tokenizer. The standalone databases have the same FTS shape so the size and timing comparison isolates tokenizer behavior rather than production relational-table overhead.

Run the opt-in tokenizer benchmarks with:

```bash
go test ./internal/indexer -run '^$' -bench 'BenchmarkRetrievalTokenizer' -benchmem
```

This experiment does not change the production `messages_fts` tokenizer or schema. It exists to decide whether a later migration is justified by retrieval quality and cost evidence.

## Benchmarks

Benchmarks are opt-in and are not run by the normal test suite. Run them explicitly with:

```bash
go test ./internal/index ./internal/indexer ./internal/codex -run '^$' -bench . -benchmem
```

The benchmark datasets cover 100, 1,000, and 10,000 sessions. Index refreshes write changed sessions in bounded batches so large initial builds avoid one SQLite commit per session. Index benchmarks report `db-bytes` alongside Go's standard `ns/op`, `B/op`, and `allocs/op` metrics. The live-search benchmark disables the optional `rg` fast path so results measure the same built-in scanner on every machine.

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
- Delegate CLI session resumption to the official Codex CLI.
- Delegate VS Code session display to the Codex extension.
- Keep discovery, parsing, and search independent from future indexing layers.
- Prefer a small, cross-platform CLI with minimal dependencies.
- Treat Ctrl+C as cooperative cancellation for indexing and search work; rollout scans and the optional ripgrep subprocess share the CLI cancellation context.
