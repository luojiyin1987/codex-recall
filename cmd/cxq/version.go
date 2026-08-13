package main

import (
	"fmt"
	"runtime/debug"
	"strings"
)

var buildVersion string

func currentVersion() string {
	moduleVersion := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		moduleVersion = info.Main.Version
	}
	return selectVersion(buildVersion, moduleVersion)
}

func selectVersion(injected, moduleVersion string) string {
	if version := strings.TrimSpace(injected); version != "" {
		return version
	}
	if version := strings.TrimSpace(moduleVersion); version != "" && version != "(devel)" {
		return version
	}
	return "devel"
}

func (c cliRunner) runVersion(args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("version does not accept arguments")
	}
	fmt.Fprintf(c.stdout, "cxq %s\n", currentVersion())
	return nil
}
