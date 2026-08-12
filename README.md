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

If Go is already installed:

```bash
go install github.com/luojiyin1987/codex-recall/cmd/cxq@latest
```

Make sure the Go binary directory (usually `$GOPATH/bin` or `~/go/bin`) is on `PATH`, then verify:

```bash
cxq help
```

## Commands

List local sessions:

```bash
cxq list
```

Search conversation text:

```bash
cxq search "Promise"
cxq search --limit 5 "annotated tag"
cxq search --project lint-md "Promise"
cxq search --source vscode "annotated tag"
cxq search --project cve-lite-cli --source vscode "tag"
```

Search is a case-insensitive literal match over user and assistant conversation text. Tool output, reasoning records, and session metadata are excluded. The first matching message from each session is shown with a compact snippet.

`--project` and `--source` are optional case-insensitive exact-match filters over the displayed `PROJECT` and `SOURCE` values. When both are supplied, both conditions must match.

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
