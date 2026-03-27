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

// ExecCommand is a variable pointing to exec.Command, allowing for mocking in tests.
var ExecCommand = exec.Command

// Init initializes the application by triggering the first bucket fetch and the spinner.
func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.spinner.Tick)
	for _, pID := range m.projectIDs {
		cmds = append(cmds, m.fetchBucketsPage(pID, ""))
	}
	return tea.Batch(cmds...)
}

func (m *Model) fetchBucketsPage(projectID string, pageToken string) tea.Cmd {
	return func() tea.Msg {
		buckets, nextToken, err := m.client.ListBucketsPage(context.Background(), projectID, pageToken, 500)
		return BucketsPageMsg{ProjectID: projectID, Buckets: buckets, NextToken: nextToken, Err: err}
	}
}

func (m *Model) fetchObjects() tea.Cmd {
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

func (m *Model) fetchObjectsPage(bucket, prefix, pageToken string) tea.Cmd {
	return func() tea.Msg {
		list, nextToken, err := m.client.ListObjectsPage(context.Background(), bucket, prefix, pageToken, 500)
		return ObjectsPageMsg{Bucket: bucket, Prefix: prefix, List: list, NextToken: nextToken, Err: err}
	}
}

func (m *Model) fetchContent(obj gcs.ObjectMetadata) tea.Cmd {
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

func (m *Model) fetchPrefixMetadataByName(name string, originalIdx int) tea.Cmd {
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

func (m *Model) fetchObjectVersions(bucket, object string) tea.Cmd {
	cachedEnabled, hasCache := m.bucketVersioningCache[bucket]

	return func() tea.Msg {
		var enabled bool
		var err error

		if hasCache {
			enabled = cachedEnabled
		} else {
			enabled, err = m.client.IsVersioningEnabled(context.Background(), bucket)
			if err != nil {
				return ObjectVersionsMsg{Bucket: bucket, ObjectName: object, Err: err}
			}
		}

		if !enabled {
			return ObjectVersionsMsg{Bucket: bucket, ObjectName: object, VersioningEnabled: false}
		}

		versions, err := m.client.ListObjectVersions(context.Background(), bucket, object)
		return ObjectVersionsMsg{
			Bucket:            bucket,
			ObjectName:        object,
			Versions:          versions,
			VersioningEnabled: true,
			Err:               err,
		}
	}
}

func (m *Model) fetchDownload(bucketName, objectName, dest, taskID string, jobNum int, isPrefix bool) tea.Cmd {
	return func() tea.Msg {
		var lastUpdate time.Time
		onProg := func(current, total int64) {
			if m.sendMsg != nil && (time.Since(lastUpdate) > 100*time.Millisecond || current == total) {
				lastUpdate = time.Now()
				m.sendMsg(DownloadProgressMsg{
					TaskID:  taskID,
					Current: current,
					Total:   total,
				})
			}
		}

		if isPrefix {
			err := m.client.DownloadPrefixAsZip(context.Background(), bucketName, objectName, dest, onProg)
			return DownloadMsg{Path: dest, TaskID: taskID, JobNum: jobNum, Err: err}
		}
		err := m.client.DownloadObject(context.Background(), bucketName, objectName, dest, onProg)
		return DownloadMsg{Path: dest, TaskID: taskID, JobNum: jobNum, Err: err}
	}
}

func (m *Model) openFile(bucketName, objectName string) tea.Cmd {
	return func() tea.Msg {
		tmpDir := os.TempDir()
		dest := filepath.Join(tmpDir, "lazygcs", bucketName, objectName)
		err := m.client.DownloadObject(context.Background(), bucketName, objectName, dest, nil)
		if err != nil {
			return FileOpenedMsg{Err: err}
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
		return FileOpenedMsg{Err: err}
	}
}

func (m *Model) editFile(bucketName, objectName string) tea.Cmd {
	tmpDir := os.TempDir()
	dest := filepath.Join(tmpDir, "lazygcs", bucketName, objectName)

	return func() tea.Msg {
		err := m.client.DownloadObject(context.Background(), bucketName, objectName, dest, nil)
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

func (m *Model) uploadFile(bucketName, objectName, srcPath string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.UploadObject(context.Background(), bucketName, objectName, srcPath)
		return UploadMsg{ObjectName: objectName, Err: err}
	}
}

func (m *Model) createBucket(projectID, bucketName string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.CreateBucket(context.Background(), projectID, bucketName)
		return CreateMsg{Name: bucketName, Err: err}
	}
}

func (m *Model) createObject(bucketName, objectName string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.CreateEmptyObject(context.Background(), bucketName, objectName)
		return CreateMsg{Name: objectName, Err: err}
	}
}

func (m *Model) startDownloadTaskDirectly(task downloadTask) (*Model, tea.Cmd) {
	jobNum := task.jobNum
	m.activeDownloads++

	var msgText string
	if progress, ok := m.jobProgress[jobNum]; ok && progress.Total > 1 {
		progress.Started++
		msgText = fmt.Sprintf("[Job #%d] Downloading %d/%d as %s...", jobNum, progress.Started, progress.Total, filepath.Base(task.dest))
	} else {
		msgText = fmt.Sprintf("[Job #%d] Downloading as %s...", jobNum, filepath.Base(task.dest))
	}

	taskID := fmt.Sprintf("dl-%s-%d", task.dest, time.Now().UnixNano())
	m.activeTasks[taskID] = Task{
		ID:       taskID,
		Name:     msgText,
		JobNum:   jobNum,
		Started:  time.Now(),
		Progress: 0,
	}

	m.activeDestinations[task.dest] = true
	cmd := m.AddMessage(LevelInfo, msgText, jobNum, taskID)
	return m, tea.Batch(cmd, m.fetchDownload(task.bucket, task.object, task.dest, taskID, jobNum, task.isPrefix))
}

func (m *Model) startDownloadTask(dest string) (*Model, tea.Cmd) {
	task := downloadTask{
		bucket:   m.pendingDownloadBucket,
		object:   m.pendingDownloadObject,
		dest:     dest,
		isPrefix: m.pendingDownloadIsPrefix,
		jobNum:   m.pendingDownloadJobNum,
	}
	return m.startDownloadTaskDirectly(task)
}

func (m *Model) processDownloadQueue() (*Model, tea.Cmd) {
	var cmds []tea.Cmd

	for len(m.downloadQueue) > 0 && m.activeDownloads < maxConcurrentDownloads {
		if m.state == viewDownloadConfirm {
			break
		}

		task := m.downloadQueue[0]

		// Check if file already exists OR is already being downloaded concurrently
		_, fileExists := os.Stat(task.dest)
		if m.activeDestinations[task.dest] {
			m.state = viewDownloadConfirm
			m.pendingDownloadBucket = task.bucket
			m.pendingDownloadObject = task.object
			m.pendingDownloadDest = task.dest
			m.pendingDownloadIsPrefix = task.isPrefix
			m.pendingDownloadJobNum = task.jobNum
			// Persistent prompt: ignore the auto-clear command
			_ = m.AddMessage(LevelWarn, fmt.Sprintf("File is actively downloading: %s - (a)bort, (r)ename, (esc) cancel batch?", filepath.Base(task.dest)), task.jobNum, "")
			m.downloadQueue = m.downloadQueue[1:]
			break
		} else if fileExists == nil {
			m.state = viewDownloadConfirm
			m.pendingDownloadBucket = task.bucket
			m.pendingDownloadObject = task.object
			m.pendingDownloadDest = task.dest
			m.pendingDownloadIsPrefix = task.isPrefix
			m.pendingDownloadJobNum = task.jobNum
			// Persistent prompt: ignore the auto-clear command
			_ = m.AddMessage(LevelWarn, fmt.Sprintf("File exists: %s - (o)verwrite, (a)bort, (r)ename, (esc) cancel batch?", filepath.Base(task.dest)), task.jobNum, "")
			m.downloadQueue = m.downloadQueue[1:]
			break
		}

		m.downloadQueue = m.downloadQueue[1:]
		var cmd tea.Cmd
		m, cmd = m.startDownloadTaskDirectly(task)
		cmds = append(cmds, cmd)
	}

	if len(cmds) == 0 {
		return m, nil
	}

	if m.state != viewDownloadConfirm {
		m.state = viewObjects
	}

	return m, tea.Batch(cmds...)
}

// StatusMessageDuration is the duration a status message is shown in the footer.
// It can be overridden in tests.
var StatusMessageDuration = 3 * time.Second

// clearStatusCmd returns a command that clears the status after a short delay.
func clearStatusCmd(id string) tea.Cmd {
	return tea.Tick(StatusMessageDuration, func(time.Time) tea.Msg {
		return ClearStatusMsg{ID: id}
	})
}

func (m *Model) triggerDebounces(previewCmd tea.Cmd, hoverBucket, hoverPrefix string) (*Model, tea.Cmd) {
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
