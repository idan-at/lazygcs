package preview

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// DockerArchivePreviewer ...
type DockerArchivePreviewer struct{}

// Priority ...
func (p *DockerArchivePreviewer) Priority() int { return 40 } // Higher priority than generic TarPreviewer (41)

// CanPreview ...
func (p *DockerArchivePreviewer) CanPreview(obj Object) bool {
	ext := strings.ToLower(filepath.Ext(obj.Name))
	name := strings.ToLower(obj.Name)
	if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
		return true
	}
	return ext == ".tar" || obj.ContentType == "application/x-tar" || obj.ContentType == "application/gzip"
}

// Preview ...
func (p *DockerArchivePreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	rc, err := client.NewReader(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	// Prevent downloading huge layers to detect if it's a docker image
	// 2MB of compressed stream should be enough to capture manifest.json and config.json
	var r io.Reader
	r = io.LimitReader(rc, 2*1024*1024)
	if strings.HasSuffix(obj.Name, ".tar.gz") || strings.HasSuffix(obj.Name, ".tgz") || obj.ContentType == "application/gzip" {
		gr, err := gzip.NewReader(r)
		if err != nil {
			return "", fmt.Errorf("failed to open gzip: %w", err)
		}
		defer func() { _ = gr.Close() }()
		r = gr
	}

	tr := tar.NewReader(r)

	// Try to find signatures of Docker or OCI archive in the first few files
	isDocker := false
	isOCI := false

	var manifestContent []byte
	var configContent []byte
	var ociIndexContent []byte

	filesExamined := 0
	for filesExamined < 20 {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		if header.Name == "manifest.json" {
			isDocker = true
			manifestContent, _ = io.ReadAll(io.LimitReader(tr, 1024*1024))
		} else if header.Name == "config.json" {
			configContent, _ = io.ReadAll(io.LimitReader(tr, 1024*1024))
		} else if strings.HasSuffix(header.Name, ".json") && isDocker && len(configContent) == 0 {
			// Naive way to grab config.json if it comes after manifest.json and has a digest name
			configContent, _ = io.ReadAll(io.LimitReader(tr, 1024*1024))
		} else if header.Name == "oci-layout" {
			isOCI = true
		} else if header.Name == "index.json" {
			ociIndexContent, _ = io.ReadAll(io.LimitReader(tr, 1024*1024))
		}

		filesExamined++
	}

	if !isDocker && !isOCI {
		return "", errors.New("not a docker or oci archive")
	}

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	var sb strings.Builder

	if isDocker {
		sb.WriteString(headerStyle.Render("Docker Image Archive") + "\n\n")

		// Parse manifest
		type dockerManifest struct {
			Config   string   `json:"Config"`
			RepoTags []string `json:"RepoTags"`
			Layers   []string `json:"Layers"`
		}
		var manifests []dockerManifest
		if len(manifestContent) > 0 {
			if err := json.Unmarshal(manifestContent, &manifests); err == nil && len(manifests) > 0 {
				m := manifests[0]
				if len(m.RepoTags) > 0 {
					sb.WriteString(keyStyle.Render("Tags:     ") + valStyle.Render(strings.Join(m.RepoTags, ", ")) + "\n")
				}
				sb.WriteString(keyStyle.Render("Layers:   ") + valStyle.Render(fmt.Sprintf("%d", len(m.Layers))) + "\n")
			}
		}

		// Parse config if available
		type dockerConfig struct {
			Architecture string `json:"architecture"`
			OS           string `json:"os"`
		}
		var cfg dockerConfig
		if len(configContent) > 0 {
			if err := json.Unmarshal(configContent, &cfg); err == nil {
				if cfg.OS != "" || cfg.Architecture != "" {
					sb.WriteString(keyStyle.Render("Platform: ") + valStyle.Render(fmt.Sprintf("%s/%s", cfg.OS, cfg.Architecture)) + "\n")
				}
			}
		}

	} else if isOCI {
		sb.WriteString(headerStyle.Render("OCI Image Archive") + "\n\n")

		type ociManifest struct {
			MediaType string `json:"mediaType"`
		}
		type ociIndex struct {
			Manifests []ociManifest `json:"manifests"`
		}

		var index ociIndex
		if len(ociIndexContent) > 0 {
			if err := json.Unmarshal(ociIndexContent, &index); err == nil {
				sb.WriteString(keyStyle.Render("Manifests: ") + valStyle.Render(fmt.Sprintf("%d", len(index.Manifests))) + "\n")
				if len(index.Manifests) > 0 {
					sb.WriteString(keyStyle.Render("MediaType: ") + valStyle.Render(index.Manifests[0].MediaType) + "\n")
				}
			}
		}
	}

	return sb.String(), nil
}

// SetWidth ...
func (p *DockerArchivePreviewer) SetWidth(_ int) {}

// DockerManifestPreviewer ...
type DockerManifestPreviewer struct{}

// Priority ...
func (p *DockerManifestPreviewer) Priority() int { return 29 } // Higher priority than ConfigPreviewer (30)

// CanPreview ...
func (p *DockerManifestPreviewer) CanPreview(obj Object) bool {
	ext := strings.ToLower(filepath.Ext(obj.Name))
	return ext == ".json"
}

// Preview ...
func (p *DockerManifestPreviewer) Preview(ctx context.Context, client GCSClient, obj Object) (string, error) {
	// Only read if it looks like a manifest file by name or size
	if obj.Size > 10*1024*1024 { // Sanity check, manifests are small
		return "", errors.New("too large to be a manifest")
	}

	content, err := client.GetObjectContent(ctx, obj.Bucket, obj.Name)
	if err != nil {
		return "", err
	}

	type minimalManifest struct {
		MediaType string `json:"mediaType"`
		Config    struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}

	var m minimalManifest
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return "", errors.New("not a valid json object")
	}

	isDocker := strings.Contains(m.MediaType, "vnd.docker.distribution.manifest")
	isOCI := strings.Contains(m.MediaType, "vnd.oci.image.manifest") || strings.Contains(m.MediaType, "vnd.oci.image.index")

	if !isDocker && !isOCI {
		return "", errors.New("not a docker or oci manifest")
	}

	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))

	var sb strings.Builder

	if isDocker {
		sb.WriteString(headerStyle.Render("Docker Manifest") + "\n\n")
	} else {
		sb.WriteString(headerStyle.Render("OCI Manifest") + "\n\n")
	}

	sb.WriteString(keyStyle.Render("MediaType: ") + valStyle.Render(m.MediaType) + "\n")
	if m.Config.Digest != "" {
		sb.WriteString(keyStyle.Render("Config Digest: ") + valStyle.Render(m.Config.Digest) + "\n")
	}

	// Also dump pretty JSON below
	sb.WriteString("\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(strings.Repeat("─", 40)) + "\n\n")

	// Prettify original content
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, []byte(content), "", "  "); err == nil {
		sb.WriteString(prettyJSON.String())
	} else {
		sb.WriteString(content)
	}

	return sb.String(), nil
}

// SetWidth ...
func (p *DockerManifestPreviewer) SetWidth(_ int) {}
