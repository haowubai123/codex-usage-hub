package client

import (
	"fmt"
	"strings"
)

func LaunchdPlist(exePath string, configPath string, label string) string {
	esc := xmlEscape
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>run</string>
		<string>--config</string>
		<string>%s</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
`, esc(label), esc(exePath), esc(configPath))
}

func WindowsCreateServiceArgs(exePath string, configPath string, serviceName string) []string {
	binPath := fmt.Sprintf("%s run --config %s", quoteWindowsArg(exePath), quoteWindowsArg(configPath))
	return []string{"create", serviceName, "binPath=", binPath, "start=", "auto"}
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}

func quoteWindowsArg(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}
