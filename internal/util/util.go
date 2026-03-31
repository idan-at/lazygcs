// Package util provides shared utilities.
package util

import (
	"regexp"
	"strings"
)

var ansiRegex = regexp.MustCompile("[\u001b\u009b][\\[\\]()#;?]*(?:(?:(?:[a-zA-Z\\d]*(?:;[-a-zA-Z\\d\\/#&.:=?%@~_]*)*)?(?:\u0007|\u001b\\\\))|(?:(?:\\d{1,4}(?:;\\d{0,4})*)?[\\dA-PR-TZcf-ntqry=><~]))")

// StripANSI removes ANSI escape codes from a string.
func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// IsBinary checks if a string contains null bytes.
func IsBinary(s string) bool {
	return strings.ContainsRune(s, '\x00')
}
