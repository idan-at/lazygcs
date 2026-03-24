package preview_test

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/idan-at/lazygcs/internal/preview"
	"gotest.tools/v3/assert"
)

func TestPDFPreviewer_Success(t *testing.T) {
	// A minimal valid PDF with 1 page and a Title info field.
	// Generated using a python script that accurately computes xref offsets.
	pdfB64 := "JVBERi0xLjQKMSAwIG9iago8PCAvVGl0bGUgKEhlbGxvIFdvcmxkKSA+PgplbmRvYmoKMiAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMyAwIFIgPj4KZW5kb2JqCjMgMCBvYmoKPDwgL1R5cGUgL1BhZ2VzIC9Db3VudCAxIC9LaWRzIFs0IDAgUl0gPj4KZW5kb2JqCjQgMCBvYmoKPDwgL1R5cGUgL1BhZ2UgL1BhcmVudCAzIDAgUiAvTWVkaWFCb3ggWzAgMCA2MTIgNzkyXSA+PgplbmRvYmoKeHJlZgowIDUKMDAwMDAwMDAwMCA2NTUzNSBmIAowMDAwMDAwMDA5IDAwMDAwIG4gCjAwMDAwMDAwNTEgMDAwMDAgbiAKMDAwMDAwMDEwMCAwMDAwMCBuIAowMDAwMDAwMTU3IDAwMDAwIG4gCnRyYWlsZXIKPDwgL1NpemUgNSAvUm9vdCAyIDAgUiAvSW5mbyAxIDAgUiA+PgpzdGFydHhyZWYKMjI4CiUlRU9GCg=="
	pdfPayload, err := base64.StdEncoding.DecodeString(pdfB64)
	assert.NilError(t, err)

	client := &mockPreviewGCSClient{content: pdfPayload}
	previewer := &preview.PDFPreviewer{}

	obj := preview.Object{Name: "test.pdf", Size: int64(len(pdfPayload))}

	assert.Assert(t, previewer.CanPreview(obj))

	out, err := previewer.Preview(context.Background(), client, obj)
	assert.NilError(t, err)

	assert.Assert(t, strings.Contains(out, "PDF Metadata:"), "Expected output to contain PDF Metadata:")
	assert.Assert(t, strings.Contains(out, "Title:"), "Expected output to contain Title:")
	assert.Assert(t, strings.Contains(out, "Hello World"), "Expected output to contain Hello World")
	assert.Assert(t, strings.Contains(out, "Pages:"), "Expected output to contain Pages:")
	assert.Assert(t, strings.Contains(out, "1"), "Expected output to contain 1")
}
