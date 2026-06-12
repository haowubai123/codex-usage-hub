package shared

import (
	"regexp"
	"strings"
)

var isoDateSuffix = regexp.MustCompile(`-\d{4}-\d{2}-\d{2}$`)
var compactDateSuffix = regexp.MustCompile(`-\d{8}$`)

func NormalizeModel(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.ReplaceAll(name, "@", "-")
	name = isoDateSuffix.ReplaceAllString(name, "")
	name = compactDateSuffix.ReplaceAllString(name, "")
	return name
}
