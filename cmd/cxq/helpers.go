package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/luojiyin1987/codex-recall/internal/codex"
)

func resolveHome(homeFlag string) (string, error) {
	if homeFlag != "" {
		return homeFlag, nil
	}
	home, err := codex.ResolveHome()
	if err != nil {
		return "", fmt.Errorf("resolve Codex home: %w", err)
	}
	return home, nil
}

func resolveResumeDir(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	info, err := os.Stat(cwd)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return cwd, nil
}

func formatDate(session codex.Session) string {
	return formatTimestamp(session.Timestamp)
}

func formatTimestamp(timestamp time.Time) string {
	if timestamp.IsZero() {
		return "-"
	}
	return timestamp.Local().Format("2006-01-02 15:04")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func hasEnvValue(env []string, key string) bool {
	return envValue(env, key) != ""
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix)
		}
	}
	return ""
}

func withEnvValue(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				result = append(result, prefix+value)
				replaced = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !replaced {
		result = append(result, prefix+value)
	}
	return result
}

func withCodexHome(env []string, home string) []string {
	const prefix = "CODEX_HOME="
	result := make([]string, 0, len(env)+1)
	replaced := false
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			if !replaced {
				result = append(result, prefix+home)
				replaced = true
			}
			continue
		}
		result = append(result, entry)
	}
	if !replaced {
		result = append(result, prefix+home)
	}
	return result
}

func vscodeConversationURL(scheme, sessionID string) (string, error) {
	if scheme != "vscode" && scheme != "vscode-insiders" {
		return "", fmt.Errorf("invalid VS Code URI scheme %q; use vscode or vscode-insiders", scheme)
	}
	if !validCodexSessionID(sessionID) {
		return "", fmt.Errorf("invalid Codex session ID %q; expected a UUID", sessionID)
	}
	return (&url.URL{
		Scheme: scheme,
		Host:   "openai.chatgpt",
		Path:   "/local/" + sessionID,
	}).String(), nil
}

func validCodexSessionID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for index := range id {
		switch index {
		case 8, 13, 18, 23:
			if id[index] != '-' {
				return false
			}
		default:
			character := id[index]
			if !((character >= '0' && character <= '9') ||
				(character >= 'a' && character <= 'f') ||
				(character >= 'A' && character <= 'F')) {
				return false
			}
		}
	}
	return true
}

func resolveOpenTarget(requested, source string) (string, error) {
	switch requested {
	case "vscode", "cli":
		return requested, nil
	case "auto":
		switch strings.ToLower(source) {
		case "vscode":
			return "vscode", nil
		case "cli":
			return "cli", nil
		default:
			return "", fmt.Errorf("session source %q has no automatic open target; use --target vscode or --target cli", emptyDash(source))
		}
	default:
		return "", fmt.Errorf("invalid open target %q; use auto, vscode, or cli", requested)
	}
}

type lookPathFunc func(string) (string, error)

type openCommand struct {
	Name string
	Args []string
	Dir  string
}

func newOpenCommand(conversationURL, goos string, env []string, lookPath lookPathFunc) (openCommand, error) {
	if goos == "linux" && (hasEnvValue(env, "WSL_DISTRO_NAME") || hasEnvValue(env, "WSL_INTEROP")) {
		if hasEnvValue(env, "VSCODE_IPC_HOOK_CLI") {
			if name, err := lookPath("code"); err == nil {
				return openCommand{
					Name: name,
					Args: []string{"--openExternal", conversationURL},
				}, nil
			}
		}

		name, err := lookPath("cmd.exe")
		if err != nil {
			return openCommand{}, fmt.Errorf("find cmd.exe: %w", err)
		}
		return openCommand{
			Name: name,
			Args: []string{"/d", "/s", "/c", "start", "", conversationURL},
			Dir:  filepath.Dir(name),
		}, nil
	}

	var opener string
	var args []string
	switch goos {
	case "windows":
		opener = "cmd.exe"
		args = []string{"/d", "/s", "/c", "start", "", conversationURL}
	case "darwin":
		opener = "open"
		args = []string{conversationURL}
	case "linux":
		opener = "xdg-open"
		args = []string{conversationURL}
	default:
		return openCommand{}, fmt.Errorf("cannot open VS Code on %s", goos)
	}

	name, err := lookPath(opener)
	if err != nil {
		return openCommand{}, fmt.Errorf("find %s: %w", opener, err)
	}
	return openCommand{Name: name, Args: args}, nil
}

func withWSLTermProgramProbeShim(env []string, lookPath lookPathFunc) ([]string, func(), error) {
	cleanup := func() {}
	if !hasEnvValue(env, "WSL_DISTRO_NAME") && !hasEnvValue(env, "WSL_INTEROP") {
		return env, cleanup, nil
	}

	realCmd, err := lookPath("cmd.exe")
	if err != nil {
		return env, cleanup, nil //nolint:nilerr // Graceful fallback when cmd.exe is unavailable.
	}
	shimDir, err := os.MkdirTemp("", "cxq-wsl-cmd-")
	if err != nil {
		return env, cleanup, err
	}
	cleanup = func() { _ = os.RemoveAll(shimDir) }

	// Codex probes Windows TERM_PROGRAM during WSL startup.
	// cmd.exe can block when its current directory is a WSL UNC path.
	const shim = `#!/bin/sh
if [ "$#" -eq 4 ] && [ "$1" = "/d" ] && [ "$2" = "/s" ] && [ "$3" = "/c" ] && [ "$4" = "set TERM_PROGRAM" ]; then
    cd "$_CXQ_REAL_CMD_DIR" || exit 1
fi
exec "$_CXQ_REAL_CMD_EXE" "$@"
`
	if err := os.WriteFile(filepath.Join(shimDir, "cmd.exe"), []byte(shim), 0o700); err != nil {
		cleanup()
		return env, func() {}, err
	}

	env = withEnvValue(env, "_CXQ_REAL_CMD_EXE", realCmd)
	env = withEnvValue(env, "_CXQ_REAL_CMD_DIR", filepath.Dir(realCmd))
	path := envValue(env, "PATH")
	if path == "" {
		path = shimDir
	} else {
		path = shimDir + string(os.PathListSeparator) + path
	}
	env = withEnvValue(env, "PATH", path)
	return env, cleanup, nil
}
