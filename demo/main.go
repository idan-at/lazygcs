// Package main provides functionality for main.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fsouza/fake-gcs-server/fakestorage"
)

func createFakeJar() []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	f, _ := w.Create("META-INF/MANIFEST.MF")
	_, _ = f.Write([]byte("Manifest-Version: 1.0\nCreated-By: lazygcs-demo\n"))

	f2, _ := w.Create("com/example/App.class")
	_, _ = f2.Write(bytes.Repeat([]byte{0xca, 0xfe, 0xba, 0xbe}, 100))

	_ = w.Close()
	return buf.Bytes()
}

func createFakeTarGz(name string, size int) []byte {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	content := bytes.Repeat([]byte("INSERT INTO table VALUES (1);\n"), size/30) // ~size bytes

	hdr := &tar.Header{
		Name: name,
		Mode: 0600,
		Size: int64(len(content)),
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(content)

	_ = tw.Close()
	_ = gw.Close()
	return buf.Bytes()
}

func main() {
	// 0. Create a temp dir for all artifacts (binary, config, downloads)
	tmpDir, err := os.MkdirTemp("", "lazygcs-demo-*")
	if err != nil {
		log.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// 1. Build lazygcs
	fmt.Println("Building lazygcs...")
	lazygcsPath := filepath.Join(tmpDir, "lazygcs")
	// #nosec G204
	buildCmd := exec.Command("go", "build", "-ldflags", "-s -w", "-o", lazygcsPath, ".")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		log.Fatalf("Failed to build lazygcs: %v", err)
	}

	// Generate mock binary content
	jarContent := createFakeJar()
	dump1Content := createFakeTarGz("db_backup_2023.sql", 50000)
	dump2Content := createFakeTarGz("db_backup_2024.sql", 60000)

	sqlPreviewContent := []byte(`-- Database Backup
CREATE TABLE users (
    id INT PRIMARY KEY,
    username VARCHAR(50) NOT NULL,
    email VARCHAR(100) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO users (id, username, email) VALUES
(1, 'admin', 'admin@example.com'),
(2, 'johndoe', 'john@example.com'),
(3, 'janedoe', 'jane@example.com');

CREATE INDEX idx_users_email ON users(email);
`)

	// 2. Start mock server
	fmt.Println("Starting mock GCS server...")
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			// demo-project buckets
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "company-assets-prod", Name: "folder/script.py"}, Content: []byte("def hello_world():\n    print(\"Hello from lazygcs!\")\n\nif __name__ == '__main__':\n    hello_world()\n")},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "company-assets-prod", Name: "folder/schema.sql"}, Content: sqlPreviewContent},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "company-assets-prod", Name: "config/settings.json"}, Content: []byte(`{"theme": "dark", "version": "1.0.0"}`)},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "company-assets-prod", Name: "css/styles.css"}, Content: []byte("body { background: #000; }")},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "company-assets-prod", Name: "index.html"}, Content: []byte("<html><body>Hello World</body></html>")},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "company-assets-prod", Name: "lib/app.jar"}, Content: jarContent},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "data-lake-raw-events", Name: "2026/03/22/events.json"}, Content: []byte(`{"event": "click", "user_id": 123}`)},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "data-lake-raw-events", Name: "2026/03/22/events2.json"}, Content: []byte(`{"event": "view", "user_id": 456}`)},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "db-backups-staging", Name: "db_backup_2023.sql.gz"}, Content: dump1Content},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "db-backups-staging", Name: "db_backup_2024.sql.gz"}, Content: dump2Content},
			{ObjectAttrs: fakestorage.ObjectAttrs{BucketName: "db-backups-staging", Name: "db_backup_2025.sql.gz"}, Content: dump1Content},
		},
		Scheme:     "http",
		Host:       "127.0.0.1",
		Port:       8081,
		PublicHost: "127.0.0.1:8081",
	})
	if err != nil {
		log.Fatalf("Failed to start mock server: %v", err)
	}
	defer server.Stop()

	// 3. Create a mock config.toml and downloads dir
	downloadDir := filepath.Join(tmpDir, "downloads")
	if err := os.MkdirAll(downloadDir, 0750); err != nil {
		log.Fatalf("Failed to create downloads dir: %v", err)
	}

	configContent := `
projects = ["demo-project"]
download_dir = "` + downloadDir + `"
fuzzy_search = true
nerd_icons = false
`
	configFile := filepath.Join(tmpDir, "config.toml")
	if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
		log.Fatalf("Failed to write config: %v", err)
	}

	// 4. Run vhs with environment variables set
	fmt.Println("Running vhs demo.tape...")
	// #nosec G204
	vhsCmd := exec.Command("vhs", filepath.Join("demo", "demo.tape"))

	// Ensure vhs uses our local lazygcs binary and our config
	var cleanEnv []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "KITTY_WINDOW_ID=") || strings.HasPrefix(e, "WEZTERM_PANE=") || strings.HasPrefix(e, "TERM=") {
			continue
		}
		cleanEnv = append(cleanEnv, e)
	}

	// Add our tmp directory to PATH so vhs finds our local "lazygcs" binary
	cleanEnv = append(cleanEnv, "PATH="+tmpDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cleanEnv = append(cleanEnv, "LAZYGCS_CONFIG="+configFile)

	// Set the emulator host, omitting the "http://" prefix
	hostURL := server.URL()
	hostURL = strings.TrimPrefix(hostURL, "http://")
	cleanEnv = append(cleanEnv, "STORAGE_EMULATOR_HOST="+hostURL)

	vhsCmd.Env = cleanEnv
	vhsCmd.Stdout = os.Stdout
	vhsCmd.Stderr = os.Stderr

	if err := vhsCmd.Run(); err != nil {
		log.Fatalf("vhs failed: %v", err)
	}

	fmt.Println("Demo recorded successfully to demo/demo.gif!")
}
