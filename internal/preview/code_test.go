package preview_test

import (
	"strings"
	"testing"

	"gotest.tools/v3/assert"
	"github.com/idan-at/lazygcs/internal/preview"
)

func TestHighlight_Go(t *testing.T) {
	content := "package main\n\nfunc main() {}"
	out, err := preview.Highlight("main.go", content)
	assert.NilError(t, err)

	// We check for the presence of the keywords.
	assert.Assert(t, strings.Contains(out, "package"))
	assert.Assert(t, strings.Contains(out, "main"))
}
