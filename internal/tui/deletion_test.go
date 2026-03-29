package tui_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_Deletion_NestedWithSiblings(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"folder/file1.txt", "folder/file2.txt"}, []string{"folder/"})
	m, _ := setupTestModel(projects, objects, "/tmp")

	// Enter bucket
	m = enterBucket(m, projects, "b1", objects)

	// Enter folder/
	m, _ = pressKey(m, 'l')
	// Simulate objects in folder/
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "folder/", List: &gcs.ObjectList{
		Objects: []gcs.ObjectMetadata{
			{Name: "folder/file1.txt", Bucket: "b1"},
			{Name: "folder/file2.txt", Bucket: "b1"},
		},
	}})

	// Initially in folder/ hovering file1.txt
	assert.Equal(t, m.FullPath(), "gs://b1/folder/file1.txt")

	// Delete file1.txt
	m, _ = updateModel(m, tui.DeleteMsg{Name: "folder/file1.txt", GoBack: false})

	// After refresh it should hover file2.txt
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "folder/", List: &gcs.ObjectList{
		Objects: []gcs.ObjectMetadata{
			{Name: "folder/file2.txt", Bucket: "b1"},
		},
	}})

	assert.Equal(t, m.FullPath(), "gs://b1/folder/file2.txt")
}

func TestModel_Deletion_NestedWithoutSiblings(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"folder/file1.txt"}, []string{"folder/"})
	m, _ := setupTestModel(projects, objects, "/tmp")

	// Enter bucket
	m = enterBucket(m, projects, "b1", objects)

	// Enter folder/
	m, _ = pressKey(m, 'l')
	// Simulate objects in folder/
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "folder/", List: &gcs.ObjectList{
		Objects: []gcs.ObjectMetadata{
			{Name: "folder/file1.txt", Bucket: "b1"},
		},
	}})

	// Initially in folder/ hovering file1.txt
	assert.Equal(t, m.FullPath(), "gs://b1/folder/file1.txt")

	// Delete file1.txt - GoBack should be true
	m, _ = updateModel(m, tui.DeleteMsg{Name: "folder/file1.txt", GoBack: true})

	// Should go back to gs://b1/
	// Simulate refresh of root objects
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: &gcs.ObjectList{
		Prefixes: []gcs.PrefixMetadata{{Name: "folder/"}},
	}})

	// In gs://b1/, it should hover folder/
	assert.Equal(t, m.FullPath(), "gs://b1/folder/")
	assert.Assert(t, strings.Contains(stripAnsi(m.View()), "📁 folder/"))
}

func TestModel_Deletion_TopLevelWithoutSiblings(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"file1.txt"}, nil)
	m, _ := setupTestModel(projects, objects, "/tmp")

	// Enter bucket
	m = enterBucket(m, projects, "b1", objects)

	// Initially in gs://b1/ hovering file1.txt
	assert.Equal(t, m.FullPath(), "gs://b1/file1.txt")

	// Delete file1.txt - GoBack should be false for top level
	m, _ = updateModel(m, tui.DeleteMsg{Name: "file1.txt", GoBack: false})

	// Should stay in gs://b1/
	// After refresh, list is empty
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: &gcs.ObjectList{}})

	assert.Equal(t, m.FullPath(), "gs://b1/")
}

func TestModel_Deletion_ConfirmLogic_NestedSingleItem(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"folder/file1.txt"}, []string{"folder/"})
	m, _ := setupTestModel(projects, objects, "/tmp")

	// 1. Setup nested folder with single item
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'l') // Enter folder/
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "folder/", List: &gcs.ObjectList{
		Objects: []gcs.ObjectMetadata{{Name: "folder/file1.txt", Bucket: "b1"}},
	}})

	// 2. Press 'x' to initiate delete
	m, _ = pressKey(m, 'x')
	assert.Assert(t, strings.Contains(stripAnsi(m.View()), "DELETE CONFIRMATION"))

	// 3. Press 'y' to confirm delete
	_, cmd := pressKey(m, 'y')

	// The cmd should be a deleteObject command that returns a DeleteMsg with GoBack: true
	msg := resolveFetchCmd(cmd)
	delMsg, ok := msg.(tui.DeleteMsg)
	assert.Assert(t, ok, "Expected DeleteMsg, got %T", msg)
	assert.Equal(t, delMsg.GoBack, true, "Should have set GoBack to true for single item in nested folder")
}

func TestModel_Deletion_ConfirmLogic_NestedWithSiblings(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"folder/file1.txt", "folder/file2.txt"}, []string{"folder/"})
	m, _ := setupTestModel(projects, objects, "/tmp")

	// 1. Setup nested folder with multiple items
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'l') // Enter folder/
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "folder/", List: &gcs.ObjectList{
		Objects: []gcs.ObjectMetadata{
			{Name: "folder/file1.txt", Bucket: "b1"},
			{Name: "folder/file2.txt", Bucket: "b1"},
		},
	}})

	// 2. Press 'x' to initiate delete
	m, _ = pressKey(m, 'x')
	assert.Assert(t, strings.Contains(stripAnsi(m.View()), "DELETE CONFIRMATION"))

	// 3. Press 'y' to confirm delete
	_, cmd := pressKey(m, 'y')

	msg := resolveFetchCmd(cmd)
	delMsg, ok := msg.(tui.DeleteMsg)
	assert.Assert(t, ok)
	assert.Equal(t, delMsg.GoBack, false, "Should NOT have set GoBack to true when siblings exist")
}

func TestModel_BucketDeletion_NoMetadataErrorAfterDelete(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1", "b2"}}}
	m, client := setupTestModel(projects, nil, "/tmp")

	// 1. Initial bucket list load
	m, _ = updateModel(m, tui.BucketsPageMsg{ProjectID: "p1", Buckets: []string{"b1", "b2"}})

	// Hover b1 (it should be at index 1 because index 0 is the project header)
	m, _ = pressKey(m, 'j')
	assert.Equal(t, m.FullPath(), "gs://b1/")

	// 2. Mock client to return 404 for b1 now (simulating it's deleted)
	client.bucketMetadataError = fmt.Errorf("googleapi: Error 404: Not Found")

	// 3. Send DeleteMsg for b1
	// In the real app, this is triggered by handleDeleteConfirmKey -> deleteBucket -> handleDeleteMsg
	m, cmd := updateModel(m, tui.DeleteMsg{Name: "b1", IsBucket: true})

	// 4. Resolve the refresh commands triggered by handleDeleteMsg -> handleRefreshKey
	// We use resolveAllFetchCmds here which we might need to define or ensure it exists
	msgs := resolveAllFetchCmds(cmd)
	for _, msg := range msgs {
		m, _ = updateModel(m, msg)
	}

	// 5. Check if error message exists.
	// The test FAILS if the error message is found (meaning the bug is present).
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "Failed to load metadata for b1") {
			t.Fatalf("Found error message for deleted bucket: %s", msg.Text)
		}
	}
}
