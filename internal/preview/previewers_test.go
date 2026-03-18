package preview_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"github.com/hamba/avro/v2"
	"github.com/hamba/avro/v2/ocf"
	"github.com/parquet-go/parquet-go"
	"gotest.tools/v3/assert"

	"github.com/idan-at/lazygcs/internal/preview"
	"github.com/idan-at/lazygcs/internal/testutil"
)

// mockPreviewGCSClient implements preview.GCSClient with a static payload.
type mockPreviewGCSClient struct {
	content []byte
}

func (m *mockPreviewGCSClient) GetObjectContent(_ context.Context, _, _ string) (string, error) {
	if len(m.content) > 1024 {
		return string(m.content[:1024]), nil
	}
	return string(m.content), nil
}

func (m *mockPreviewGCSClient) NewReader(_ context.Context, _, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(m.content)), nil
}

func (m *mockPreviewGCSClient) NewReaderAt(_ context.Context, _, _ string) io.ReaderAt {
	return bytes.NewReader(m.content)
}

func TestPreviewers(t *testing.T) {
	// Pre-generate Parquet data
	type Row struct {
		ID   int    `parquet:"id"`
		Name string `parquet:"name"`
	}
	pqBuf := new(bytes.Buffer)
	pqWriter := parquet.NewGenericWriter[Row](pqBuf)
	_, _ = pqWriter.Write([]Row{{ID: 1, Name: "Alice"}, {ID: 2, Name: "Bob"}})
	_ = pqWriter.Close()

	// Pre-generate Avro data
	schema, _ := avro.Parse(`{"type":"record","name":"test","fields":[{"name":"id","type":"int"},{"name":"name","type":"string"}]}`)
	avroBuf := new(bytes.Buffer)
	enc, _ := ocf.NewEncoder(schema.String(), avroBuf)
	_ = enc.Encode(map[string]any{"id": 1, "name": "Alice"})
	_ = enc.Close()

	// Pre-generate Zip data
	zipData := testutil.CreateMockZip(t, map[string]string{"file_in_zip.txt": "inner content"})

	// Pre-generate Tar data
	tarDocker := testutil.CreateMockTar(t, map[string]string{
		"manifest.json": `[{"Config":"config.json","RepoTags":["test:latest"],"Layers":["layer.tar"]}]`,
		"config.json":   `{"architecture":"amd64","os":"linux"}`,
	})
	tarOCI := testutil.CreateMockTar(t, map[string]string{
		"oci-layout": `{"imageLayoutVersion": "1.0.0"}`,
		"index.json": `{"manifests": [{"mediaType": "application/vnd.oci.image.manifest.v1+json"}]}`,
	})
	tarFallback := testutil.CreateMockTar(t, map[string]string{"random.txt": "hello world"})

	pdfB64 := "JVBERi0xLjQKJcOkw7zDtsOfCjIgMCBvYmoKPDwvTGVuZ3RoIDMgMCBSL0ZpbHRlci9GbGF0ZURlY29kZT4+CnN0cmVhbQp4nDPQM1Qo5ypUMFAwALJMLU31jBQsTAz1LBSK0osSQTz9xJLMYoXSvOSSzOScxLz01DyF/JzEnNRihdzMvNQShZJ8QyMgx8DIQKEkM0XBRM8QwL12GgplbmRzdHJlYW0KZW5kb2JqCgozIDAgb2JqCjc5CmVuZG9iagoKNCAwIG9iago8PC9UeXBlL1BhZ2UvTWVkaWFCb3ggWzAgMCA1OTUuMjggODQxLjg5XS9SZXNvdXJjZXM8PC9Gb250PDwvRjEgNSAwIFI+Pj4+L0NvbnRlbnRzIDIgMCBSL1BhcmVudCAxIDAgUj4+CmVuZG9iagoKNSAwIG9iago8PC9UeXBlL0ZvbnQvU3VidHlwZS9UeXBlMS9CYXNlRm9udC9IZWx2ZXRpY2EvRW5jb2RpbmcvV2luQW5zaUVuY29kaW5nPj4KZW5kb2JqCgoxIDAgb2JqCjw8L1R5cGUvUGFnZXMvS2lkc1s0IDAgUl0vQ291bnQgMT4+CmVuZG9iagoKNCAwIG9iago8PC9UeXBlL0NhdGFsb2cvUGFnZXMgMSAwIFI+PgplbmRvYmoKCjYgMCBvYmoKPDwvUHJvZHVjZXIoZ29mcGRmIDEuMTYuMikvQ3JlYXRpb25EYXRlKEQ6MjAyMzA4MjgxNjQ0NDRaKT4+CmVuZG9iagoKeHJlZgowIDcKMDAwMDAwMDAwMCA2NTUzNSBmIAowMDAwMDAwMjk5IDAwMDAwIG4gCjAwMDAwMDAwMTUgMDAwMDAgbiAKMDAwMDAwMDE2NSAwMDAwMCBuIAowMDAwMDAwMTg2IDAwMDAwIG4gCjAwMDAwMDAyOTkgMDAwMDAgbiAKMDAwMDAwMDQxNiAwMDAwMCBuIAp0cmFpbGVyCjw8L1NpemUgNy9Sb290IDQgMCBSL0luZm8gNiAwIFI+PgpzdGFydHhyZWYKNTA3CiUlRU9GCg=="
	pdfPayload, _ := base64.StdEncoding.DecodeString(pdfB64)

	testCases := []struct {
		name        string
		obj         preview.Object
		content     []byte
		previewer   preview.Previewer
		contains    []string
		notContains []string
		expectErr   bool
	}{
		{
			name:      "Markdown",
			obj:       preview.Object{Name: "README.md", Size: int64(len("# Hello\n\nThis is **markdown**"))},
			content:   []byte("# Hello\n\nThis is **markdown**"),
			previewer: preview.NewMarkdownPreviewer(80),
			contains:  []string{"Hello", "markdown"},
		},
		{
			name:      "Zip",
			obj:       preview.Object{Name: "test.zip", Size: int64(len(zipData))},
			content:   zipData,
			previewer: &preview.ZipPreviewer{},
			contains:  []string{"file_in_zip.txt"},
		},
		{
			name:      "JSON",
			obj:       preview.Object{Name: "data.json", ContentType: "application/json", Size: int64(len(`{"name":"test","value":123}`))},
			content:   []byte(`{"name":"test","value":123}`),
			previewer: &preview.ConfigPreviewer{},
			contains:  []string{"{", "name", "test", "value"},
		},
		{
			name:      "CSV",
			obj:       preview.Object{Name: "data.csv", Size: int64(len("id,name,city\n1,Alice,London\n2,Bob,Paris"))},
			content:   []byte("id,name,city\n1,Alice,London\n2,Bob,Paris"),
			previewer: &preview.DataPreviewer{},
			contains:  []string{"id", "Alice", "Paris"},
		},
		{
			name:      "Parquet",
			obj:       preview.Object{Name: "data.parquet", Size: int64(pqBuf.Len())},
			content:   pqBuf.Bytes(),
			previewer: &preview.DataPreviewer{},
			contains:  []string{"Alice", "id"},
		},
		{
			name:      "YAML",
			obj:       preview.Object{Name: "config.yaml", Size: int64(len("app:\n  name: lazygcs\n  enabled: true"))},
			content:   []byte("app:\n  name: lazygcs\n  enabled: true"),
			previewer: &preview.ConfigPreviewer{},
			contains:  []string{"lazygcs", "enabled"},
		},
		{
			name:      "TOML",
			obj:       preview.Object{Name: "settings.toml", Size: int64(len("[server]\nport = 8080\nhost = \"localhost\""))},
			content:   []byte("[server]\nport = 8080\nhost = \"localhost\""),
			previewer: &preview.ConfigPreviewer{},
			contains:  []string{"8080", "localhost"},
		},
		{
			name:      "Avro",
			obj:       preview.Object{Name: "data.avro", Size: int64(avroBuf.Len())},
			content:   avroBuf.Bytes(),
			previewer: &preview.DataPreviewer{},
			contains:  []string{"Alice", "Avro Schema"},
		},
		{
			name:      "Logs",
			obj:       preview.Object{Name: "app.log", Size: int64(len("INFO: starting up\nERROR: failed to connect\nWARN: retrying"))},
			content:   []byte("INFO: starting up\nERROR: failed to connect\nWARN: retrying"),
			previewer: &preview.LogPreviewer{},
			contains:  []string{"ERROR", "failed to connect"},
		},
		{
			name:      "PDF",
			obj:       preview.Object{Name: "test.pdf", Size: int64(len(pdfPayload))},
			content:   pdfPayload,
			previewer: &preview.PDFPreviewer{},
			expectErr: true,
		},
		{
			name:      "Conf",
			obj:       preview.Object{Name: "server.conf", Size: int64(len("listen = 80\nserver_name = localhost"))},
			content:   []byte("listen = 80\nserver_name = localhost"),
			previewer: &preview.ConfigPreviewer{},
			contains:  []string{"listen", "80"},
		},
		{
			name:      "Properties",
			obj:       preview.Object{Name: "app.properties", Size: int64(len("app.version=1.2.3\napp.env=prod"))},
			content:   []byte("app.version=1.2.3\napp.env=prod"),
			previewer: &preview.ConfigPreviewer{},
			contains:  []string{"version", "1.2.3"},
		},
		{
			name:      "Shell Script",
			obj:       preview.Object{Name: "script.sh", Size: int64(len("#!/bin/bash\necho 'hello world'"))},
			content:   []byte("#!/bin/bash\necho 'hello world'"),
			previewer: &preview.CodePreviewer{},
			contains:  []string{"echo", "hello world", "\x1b["},
		},
		{
			name:      "Shell Shebang",
			obj:       preview.Object{Name: "myscript", Size: int64(len("#!/bin/zsh\necho 'zsh rules'"))},
			content:   []byte("#!/bin/zsh\necho 'zsh rules'"),
			previewer: &preview.TextPreviewer{},
			contains:  []string{"echo", "zsh rules"},
		},
		{
			name:      "Tar Docker",
			obj:       preview.Object{Name: "image.tar", Size: int64(len(tarDocker))},
			content:   tarDocker,
			previewer: &preview.DockerArchivePreviewer{},
			contains:  []string{"Docker Image", "linux/amd64"},
		},
		{
			name:      "Tar OCI",
			obj:       preview.Object{Name: "oci.tar", Size: int64(len(tarOCI))},
			content:   tarOCI,
			previewer: &preview.DockerArchivePreviewer{},
			contains:  []string{"OCI Image"},
		},
		{
			name:      "Tar Fallback",
			obj:       preview.Object{Name: "random.tar", Size: int64(len(tarFallback))},
			content:   tarFallback,
			previewer: &preview.TarPreviewer{},
			contains:  []string{"Archive contents", "random.txt"},
		},
		{
			name:      "JSON Docker Manifest",
			obj:       preview.Object{Name: "manifest.json", Size: int64(len(`{"mediaType": "application/vnd.docker.distribution.manifest.v2+json", "config": {"digest": "sha256:12345"}}`))},
			content:   []byte(`{"mediaType": "application/vnd.docker.distribution.manifest.v2+json", "config": {"digest": "sha256:12345"}}`),
			previewer: &preview.DockerManifestPreviewer{},
			contains:  []string{"Docker Manifest"},
		},
		{
			name:      "JSON OCI Manifest",
			obj:       preview.Object{Name: "index.json", Size: int64(len(`{"mediaType": "application/vnd.oci.image.manifest.v1+json", "config": {"digest": "sha256:67890"}}`))},
			content:   []byte(`{"mediaType": "application/vnd.oci.image.manifest.v1+json", "config": {"digest": "sha256:67890"}}`),
			previewer: &preview.DockerManifestPreviewer{},
			contains:  []string{"OCI Manifest"},
		},
		{
			name:        "JSON Generic Fallback (ConfigPreviewer)",
			obj:         preview.Object{Name: "generic.json", Size: int64(len(`{"foo": "bar"}`))},
			content:     []byte(`{"foo": "bar"}`),
			previewer:   &preview.ConfigPreviewer{},
			contains:    []string{"foo", "bar"},
			notContains: []string{"Docker", "OCI"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := &mockPreviewGCSClient{content: tc.content}

			// Verify CanPreview logic
			assert.Assert(t, tc.previewer.CanPreview(tc.obj), "previewer %T rejected object %q", tc.previewer, tc.obj.Name)

			// Execute Preview
			out, err := tc.previewer.Preview(context.Background(), client, tc.obj)
			if tc.expectErr {
				assert.Assert(t, err != nil, "expected error for %s", tc.name)
				return
			}
			assert.NilError(t, err)

			// Assert output contains expected strings
			for _, exp := range tc.contains {
				if !strings.Contains(out, exp) {
					t.Errorf("expected output to contain %q\nOutput: %s", exp, out)
				}
			}

			for _, exp := range tc.notContains {
				if strings.Contains(out, exp) {
					t.Errorf("expected output NOT to contain %q\nOutput: %s", exp, out)
				}
			}
		})
	}
}
