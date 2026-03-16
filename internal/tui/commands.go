// Package tui provides functionality for tui.
package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/preview"
)

var ExecCommand = exec.Command

// Init initializes the application by triggering the first bucket fetch and the spinner.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.spinner.Tick)
	for _, pID := range m.projectIDs {
		cmds = append(cmds, m.fetchBucketsPage(pID, ""))
	}
	return tea.Batch(cmds...)
}

func (m Model) fetchBucketsPage(projectID string, pageToken string) tea.Cmd {
	return func() tea.Msg {
		buckets, nextToken, err := m.client.ListBucketsPage(context.Background(), projectID, pageToken, 500)
		return BucketsPageMsg{ProjectID: projectID, Buckets: buckets, NextToken: nextToken, Err: err}
	}
}

func (m Model) fetchObjects() tea.Cmd {
	bucket := m.currentBucket
	prefix := m.currentPrefix

	cacheKey := bucket + "::" + prefix
	if cached, ok := m.listCache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
		return func() tea.Msg {
			return ObjectsMsg{Bucket: bucket, Prefix: prefix, List: cached.List, Err: nil}
		}
	}

	return m.fetchObjectsPage(bucket, prefix, "")
}

func (m Model) fetchObjectsPage(bucket, prefix, pageToken string) tea.Cmd {
	return func() tea.Msg {
		list, nextToken, err := m.client.ListObjectsPage(context.Background(), bucket, prefix, pageToken, 500)
		return ObjectsPageMsg{Bucket: bucket, Prefix: prefix, List: list, NextToken: nextToken, Err: err}
	}
}

func (m Model) fetchContent(obj gcs.ObjectMetadata) tea.Cmd {
	bucketName := m.currentBucket
	objectName := obj.Name
	cacheKey := bucketName + "::" + objectName
	if cached, ok := m.contentCache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
		return func() tea.Msg {
			return ContentMsg{ObjectName: objectName, Content: cached.Content, Err: nil}
		}
	}

	return func() tea.Msg {
		pObj := preview.Object{
			Bucket:      bucketName,
			Name:        objectName,
			Size:        obj.Size,
			ContentType: obj.ContentType,
		}
		content, err := m.previewRegistry.GetPreview(context.Background(), m.client, pObj)
		return ContentMsg{ObjectName: objectName, Content: content, Err: err}
	}
}

func (m Model) fetchPrefixMetadataByName(name string, originalIdx int) tea.Cmd {
	bucket := m.currentBucket
	prefix := m.currentPrefix

	cacheKey := bucket + "::" + name
	if cached, ok := m.metadataCache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
		return func() tea.Msg {
			return MetadataMsg{Bucket: bucket, Prefix: prefix, PrefixIndex: originalIdx, Metadata: cached.Metadata, Err: nil}
		}
	}

	return func() tea.Msg {
		meta, err := m.client.GetObjectMetadata(context.Background(), bucket, name)
		return MetadataMsg{Bucket: bucket, Prefix: prefix, PrefixIndex: originalIdx, Metadata: meta, Err: err}
	}
}

func (m Model) fetchDownload(bucketName, objectName, dest string, isPrefix bool) tea.Cmd {
	return func() tea.Msg {
		if isPrefix {
			err := m.client.DownloadPrefixAsZip(context.Background(), bucketName, objectName, dest)
			return DownloadMsg{Path: dest, Err: err}
		}
		err := m.client.DownloadObject(context.Background(), bucketName, objectName, dest)
		return DownloadMsg{Path: dest, Err: err}
	}
}

func (m Model) openFile(bucketName, objectName string) tea.Cmd {
	return func() tea.Msg {
		tmpDir := os.TempDir()
		dest := filepath.Join(tmpDir, "lazygcs", bucketName, objectName)
		err := m.client.DownloadObject(context.Background(), bucketName, objectName, dest)
		if err != nil {
			return DownloadMsg{Err: err}
		}

		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = ExecCommand("open", dest)
		case "windows":
			cmd = ExecCommand("rundll32", "url.dll,FileProtocolHandler", dest)
		default: // linux, bsd, etc.
			cmd = ExecCommand("xdg-open", dest)
		}

		err = cmd.Start()
		return DownloadMsg{Path: dest, Err: err}
	}
}

func (m Model) editFile(bucketName, objectName string) tea.Cmd {
	tmpDir := os.TempDir()
	dest := filepath.Join(tmpDir, "lazygcs", bucketName, objectName)

	return func() tea.Msg {
		err := m.client.DownloadObject(context.Background(), bucketName, objectName, dest)
		if err != nil {
			return EditorFinishedMsg{Err: err}
		}

		info, err := os.Stat(dest)
		if err != nil {
			return EditorFinishedMsg{Err: err}
		}
		modTime := info.ModTime()

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim"
		}

		c := ExecCommand(editor, dest)
		return tea.ExecProcess(c, func(err error) tea.Msg {
			return EditorFinishedMsg{
				TempPath:        dest,
				OriginalModTime: modTime,
				Err:             err,
			}
		})()
	}
}

func (m Model) uploadFile(bucketName, objectName, srcPath string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.UploadObject(context.Background(), bucketName, objectName, srcPath)
		return UploadMsg{ObjectName: objectName, Err: err}
	}
}

func (m Model) processDownloadQueue() (Model, tea.Cmd) {
	if len(m.downloadQueue) == 0 {
		return m, nil
	}

	task := m.downloadQueue[0]
	m.downloadQueue = m.downloadQueue[1:]

	// Check if file already exists
	if _, err := os.Stat(task.dest); err == nil {
		m.state = viewDownloadConfirm
		m.pendingDownloadBucket = task.bucket
		m.pendingDownloadObject = task.object
		m.pendingDownloadDest = task.dest
		m.pendingDownloadIsPrefix = task.isPrefix
		m.status = fmt.Sprintf("File exists: %s - (o)verwrite, (a)bort, (r)ename?", filepath.Base(task.dest))
		return m, nil
	}

	if m.downloadTotal > 1 {
		m.status = fmt.Sprintf("Downloading %d/%d: %s...", m.downloadFinished+1, m.downloadTotal, filepath.Base(task.dest))
	} else {
		m.status = fmt.Sprintf("Downloading %s...", filepath.Base(task.dest))
	}
	return m, m.fetchDownload(task.bucket, task.object, task.dest, task.isPrefix)
}

// clearStatusCmd returns a command that clears the status after a short delay.
func clearStatusCmd() tea.Cmd {
	return tea.Tick(time.Second*3, func(time.Time) tea.Msg {
		return ClearStatusMsg{}
	})
}

func (m Model) triggerDebounces(previewCmd tea.Cmd, hoverBucket, hoverPrefix string) (Model, tea.Cmd) {
	m.cursorVersion++
	cv := m.cursorVersion

	var cmds []tea.Cmd

	if previewCmd != nil {
		cmds = append(cmds, tea.Tick(150*time.Millisecond, func(_ time.Time) tea.Msg {
			return DebouncePreviewMsg{CursorVersion: cv, FetchCmd: previewCmd}
		}))
	}

	if hoverBucket != "" {
		hoverCmd := func() tea.Msg {
			cacheKey := hoverBucket + "::" + hoverPrefix
			if cached, ok := m.listCache[cacheKey]; ok && time.Now().Before(cached.ExpiresAt) {
				return nil
			}
			list, err := m.client.ListObjects(context.Background(), hoverBucket, hoverPrefix)
			return HoverPrefetchMsg{Bucket: hoverBucket, Prefix: hoverPrefix, List: list, Err: err}
		}

		cmds = append(cmds, tea.Tick(300*time.Millisecond, func(_ time.Time) tea.Msg {
			return HoverPrefetchTickMsg{CursorVersion: cv, FetchCmd: hoverCmd}
		}))
	}

	return m, tea.Batch(cmds...)
}
