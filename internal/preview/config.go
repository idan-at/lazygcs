package preview

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

// ConfigPreviewer ...
type ConfigPreviewer struct{}

// Priority ...
func (p *ConfigPreviewer) Priority() int { return 30 }

// CanPreview ...
func (p *ConfigPreviewer) CanPreview(obj Object) bool {
	ext := strings.ToLower(filepath.Ext(obj.Name))
	return ext == ".json" || ext == ".yaml" || ext == ".yml" || ext == ".toml" ||
		ext == ".conf" || ext == ".properties" ||
		obj.ContentType == "application/json" || obj.ContentType == "application/x-yaml"
}

// Preview ...
func (p *ConfigPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	// Read first 20KB for config files
	limit := int64(20 * 1024)
	if obj.Size < limit {
		limit = obj.Size
	}
	buf := make([]byte, limit)
	n, err := io.ReadFull(rc, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	content := buf[:n]

	ext := strings.ToLower(filepath.Ext(obj.Name))
	var indented string

	switch {
	case ext == ".json" || obj.ContentType == "application/json":
		var data any
		if err := json.Unmarshal(content, &data); err == nil {
			if b, err := json.MarshalIndent(data, "", "  "); err == nil {
				indented = string(b)
			}
		}
	case ext == ".yaml" || ext == ".yml" || obj.ContentType == "application/x-yaml":
		var data any
		if err := yaml.Unmarshal(content, &data); err == nil {
			if b, err := yaml.Marshal(&data); err == nil {
				indented = string(b)
			}
		}
	case ext == ".toml":
		var data any
		if err := toml.Unmarshal(content, &data); err == nil {
			if b, err := toml.Marshal(data); err == nil {
				indented = string(b)
			}
		}
	}

	if indented == "" {
		indented = string(content)
	}

	return Highlight(obj.Name, indented)
}

// SetWidth ...
func (p *ConfigPreviewer) SetWidth(_ int) {}
