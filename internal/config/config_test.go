package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/idan-at/lazygcs/internal/config"
	"gotest.tools/v3/assert"
)

func createConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(content), 0600)
	assert.NilError(t, err)
	return path
}

func TestLoad(t *testing.T) {
	home, err := os.UserHomeDir()
	assert.NilError(t, err)
	defaultDownload := filepath.Join(home, "Downloads")

	tests := []struct {
		name        string
		content     string
		fileMissing bool
		wantErr     bool
		expected    *config.Config
	}{
		{
			name:        "NoFile",
			fileMissing: true,
			wantErr:     true,
		},
		{
			name:    "MalformedTOML",
			content: `projects = ["p1"`,
			wantErr: true,
		},
		{
			name:    "BasicProjects",
			content: `projects = ["p1", "p2"]`,
			expected: &config.Config{
				Projects:    []string{"p1", "p2"},
				DownloadDir: defaultDownload,
				FuzzySearch: true,
			},
		},
		{
			name:    "ProjectsWithWhitespace",
			content: `projects = [" p1 ", " p2 "]`,
			expected: &config.Config{
				Projects:    []string{"p1", "p2"},
				DownloadDir: defaultDownload,
				FuzzySearch: true,
			},
		},
		{
			name:    "OverrideDownloadDir",
			content: `download_dir = "/tmp/custom_downloads"`,
			expected: &config.Config{
				Projects:    []string{},
				DownloadDir: "/tmp/custom_downloads",
				FuzzySearch: true,
			},
		},
		{
			name: "DisableFuzzySearch",
			content: `
projects = ["p1"]
fuzzy_search = false
`,
			expected: &config.Config{
				Projects:    []string{"p1"},
				DownloadDir: defaultDownload,
				FuzzySearch: false,
			},
		},
		{
			name: "OverrideNerdIcons",
			content: `
projects = ["p1"]
nerd_icons = true
`,
			expected: &config.Config{
				Projects:    []string{"p1"},
				DownloadDir: defaultDownload,
				FuzzySearch: true,
				NerdIcons:   true,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := "non-existent.toml"
			if !tt.fileMissing {
				path = createConfigFile(t, tt.content)
			}

			cfg, err := config.Load(path)
			if tt.wantErr {
				assert.Assert(t, err != nil)
				return
			}
			assert.NilError(t, err)
			if len(tt.expected.Projects) == 0 {
				assert.Equal(t, len(cfg.Projects), 0)
			} else {
				assert.DeepEqual(t, cfg.Projects, tt.expected.Projects)
			}
			assert.Equal(t, cfg.DownloadDir, tt.expected.DownloadDir)
			assert.Equal(t, cfg.FuzzySearch, tt.expected.FuzzySearch)
			assert.Equal(t, cfg.NerdIcons, tt.expected.NerdIcons)
		})
	}
}

func TestLoad_PermissionError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	err := os.WriteFile(path, []byte(`projects = ["p1"]`), 0200)
	assert.NilError(t, err)

	_, err = config.Load(path)
	assert.Assert(t, err != nil)
	assert.Assert(t, os.IsPermission(err))
}

func TestInitConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("LAZYGCS_CONFIG", "")

	t.Run("CustomPath", func(t *testing.T) {
		path := filepath.Join(homeDir, "custom", "config.toml")
		err := config.InitConfig(path, []string{"proj1"})
		assert.NilError(t, err)

		_, err = os.Stat(path)
		assert.NilError(t, err)

		cfg, err := config.Load(path)
		assert.NilError(t, err)
		assert.DeepEqual(t, cfg.Projects, []string{"proj1"})

		err = config.InitConfig(path, []string{"proj2"})
		assert.Assert(t, err != nil)
		assert.ErrorContains(t, err, "already exists")
	})

	t.Run("DefaultPath", func(t *testing.T) {
		err := config.InitConfig("", []string{"proj2"})
		assert.NilError(t, err)

		defaultPath := filepath.Join(homeDir, ".config", "lazygcs", "config.toml")
		_, err = os.Stat(defaultPath)
		assert.NilError(t, err)

		cfg, err := config.Load(defaultPath)
		assert.NilError(t, err)
		assert.DeepEqual(t, cfg.Projects, []string{"proj2"})
	})
}
