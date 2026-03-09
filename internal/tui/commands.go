package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
)

// Init initializes the application by triggering the first bucket fetch.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		projects, err := m.client.ListBuckets(context.Background(), m.projectIDs)
		return BucketsMsg{Projects: projects, Err: err}
	}
}

func (m Model) fetchObjects() tea.Cmd {
	bucket := m.currentBucket
	prefix := m.currentPrefix
	return func() tea.Msg {
		list, err := m.client.ListObjects(context.Background(), bucket, prefix)
		return ObjectsMsg{Bucket: bucket, Prefix: prefix, List: list, Err: err}
	}
}

func (m Model) fetchContent(bucketName, objectName string) tea.Cmd {
	return func() tea.Msg {
		content, err := m.client.GetObjectContent(context.Background(), bucketName, objectName)
		return ContentMsg{ObjectName: objectName, Content: content, Err: err}
	}
}

func (m Model) fetchPrefixMetadataByName(name string, originalIdx int) tea.Cmd {
	bucket := m.currentBucket
	prefix := m.currentPrefix
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

func (m *Model) processDownloadQueue() tea.Cmd {
	if len(m.downloadQueue) == 0 {
		return nil
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
		return nil
	}

	if m.downloadTotal > 1 {
		m.status = fmt.Sprintf("Downloading %d/%d: %s...", m.downloadFinished+1, m.downloadTotal, filepath.Base(task.dest))
	} else {
		m.status = fmt.Sprintf("Downloading %s...", filepath.Base(task.dest))
	}
	return m.fetchDownload(task.bucket, task.object, task.dest, task.isPrefix)
}