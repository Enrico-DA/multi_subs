package multisubs

import "strings"

// shellQuote renders s as a single-quoted POSIX shell word. Test scripts embed
// temporary paths, which may contain characters the shell would otherwise split.
func shellQuote(s string) string {
	s = strings.ReplaceAll(s, `'`, `'\''`)
	return "'" + s + "'"
}
