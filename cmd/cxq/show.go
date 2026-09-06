package main

import (
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/luojiyin1987/codex-recall/internal/codex"
)

func (c cliRunner) runShow(args []string) error {
	flags := flag.NewFlagSet("show", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("show: %w", err)
	}
	if flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("usage: cxq show [--home PATH] SESSION")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	session, err := codex.NewCatalog(home).Resolve(flags.Arg(0))
	if err != nil {
		return err
	}
	messages, err := codex.ReadConversation(session.Path)
	if err != nil {
		return fmt.Errorf("read conversation: %w", err)
	}

	writer := tabwriter.NewWriter(c.stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintf(writer, "SESSION\t%s\n", session.ID)
	fmt.Fprintf(writer, "DATE\t%s\n", formatDate(session))
	fmt.Fprintf(writer, "PROJECT\t%s\n", session.Project())
	fmt.Fprintf(writer, "SOURCE\t%s\n", session.Source)
	fmt.Fprintf(writer, "CWD\t%s\n", emptyDash(session.CWD))
	fmt.Fprintf(writer, "PATH\t%s\n", session.Path)
	if err := writer.Flush(); err != nil {
		return err
	}

	if len(messages) == 0 {
		fmt.Fprintln(c.stdout, "\n(no user/assistant messages)")
		return nil
	}
	for _, message := range messages {
		label := message.Role
		if !message.Timestamp.IsZero() {
			label += " " + message.Timestamp.Local().Format("2006-01-02 15:04:05")
		}
		fmt.Fprintf(c.stdout, "\n[%s]\n%s\n", label, message.Text)
	}
	return nil
}

func (c cliRunner) runResume(args []string) error {
	flags := flag.NewFlagSet("resume", flag.ContinueOnError)
	flags.SetOutput(c.stderr)
	homeFlag := flags.String("home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("resume: %w", err)
	}
	if flags.NArg() != 1 || strings.TrimSpace(flags.Arg(0)) == "" {
		return fmt.Errorf("usage: cxq resume [--home PATH] SESSION")
	}

	home, err := resolveHome(*homeFlag)
	if err != nil {
		return err
	}
	session, err := codex.NewCatalog(home).Resolve(flags.Arg(0))
	if err != nil {
		return err
	}
	return c.resumeSession(home, session)
}
