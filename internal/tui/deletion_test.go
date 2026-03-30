package tui_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/idan-at/lazygcs/internal/gcs"
	"github.com/idan-at/lazygcs/internal/tui"
	"gotest.tools/v3/assert"
)

func TestModel_Deletion_NestedWithSiblings(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"folder/file1.txt", "folder/file2.txt"}, []string{"folder/"})
	// objects helper might not set Bucket, let's ensure it's there
	for i := range objects.Objects {
		objects.Objects[i].Bucket = "b1"
	}
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
	for i := range objects.Objects {
		objects.Objects[i].Bucket = "b1"
	}
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

	// Delete file1.txt - should trigger back navigation
	m, _ = pressKey(m, 'x')
	_, cmd := pressKey(m, 'y')

	// Simulate the DeleteMsg coming back
	msg := resolveFetchCmd(cmd)
	_, backCmd := updateModel(m, msg)

	// Resolve the back navigation
	m, _ = updateModel(m, backCmd)

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
	for i := range objects.Objects {
		objects.Objects[i].Bucket = "b1"
	}
	m, _ := setupTestModel(projects, objects, "/tmp")

	// Enter bucket
	m = enterBucket(m, projects, "b1", objects)

	// Initially in gs://b1/ hovering file1.txt
	assert.Equal(t, m.FullPath(), "gs://b1/file1.txt")

	// Delete file1.txt
	m, _ = updateModel(m, tui.DeleteMsg{Name: "file1.txt", GoBack: false})

	// Should stay in gs://b1/
	// After refresh, list is empty
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "", List: &gcs.ObjectList{}})

	assert.Equal(t, m.FullPath(), "gs://b1/")
}

func TestModel_Deletion_ConfirmLogic_NestedSingleItem(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"folder/file1.txt"}, []string{"folder/"})
	for i := range objects.Objects {
		objects.Objects[i].Bucket = "b1"
	}
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

	// The cmd should be a deleteObject command. We'll simulate its completion.
	msg := resolveFetchCmd(cmd)
	_, backCmd := updateModel(m, msg)

	// It should trigger navigation back because it was the only item
	assert.Assert(t, backCmd != nil, "Should have returned a command to go back")
}

func TestModel_Deletion_ConfirmLogic_NestedWithSiblings(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"folder/file1.txt", "folder/file2.txt"}, []string{"folder/"})
	for i := range objects.Objects {
		objects.Objects[i].Bucket = "b1"
	}
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
	m, refreshCmd := updateModel(m, msg)
	assert.Assert(t, refreshCmd != nil)
	// After single deletion finishes, it triggers refresh, clearing objects temporarily
	assert.Equal(t, m.FullPath(), "gs://b1/folder/")
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
	m, cmd := updateModel(m, tui.DeleteMsg{Name: "b1", IsBucket: true})

	// 4. Resolve the refresh commands triggered by handleDeleteMsg -> handleRefreshKey
	msgs := resolveAllFetchCmds(cmd)
	for _, msg := range msgs {
		m, _ = updateModel(m, msg)
	}

	// 5. Check if error message exists.
	for _, msg := range m.Messages() {
		if strings.Contains(msg.Text, "Failed to load metadata for b1") {
			t.Fatalf("Found error message for deleted bucket: %s", msg.Text)
		}
	}
}

func TestModel_MultiDelete_Supported(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	for i := range objects.Objects {
		objects.Objects[i].Bucket = "b1"
	}
	m, _ := setupTestModel(projects, objects, "/tmp")

	// Enter bucket
	m = enterBucket(m, projects, "b1", objects)

	// Select obj1
	m, _ = pressKey(m, ' ')
	// Move to obj2
	m, _ = pressKey(m, 'j')
	// Select obj2
	m, _ = pressKey(m, ' ')

	// Press 'x' to delete
	m, _ = pressKey(m, 'x')

	// Verify confirmation view shows "2 selected items"
	assert.Assert(t, strings.Contains(stripAnsi(m.View()), "2 selected items"))

	// Press 'y' to confirm
	_, cmd := pressKey(m, 'y')

	// Verify cmd is a batch of 2 deletion commands
	batch, ok := cmd().(tea.BatchMsg)
	assert.Assert(t, ok, "Expected BatchMsg, got %T", cmd())
	assert.Equal(t, len(batch), 2, "Expected 2 deletion commands in batch")
}

func TestModel_MultiDelete_GoBack(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"folder/file1.txt", "folder/file2.txt"}, []string{"folder/"})
	for i := range objects.Objects {
		objects.Objects[i].Bucket = "b1"
	}
	m, _ := setupTestModel(projects, objects, "/tmp")

	// Enter bucket
	m = enterBucket(m, projects, "b1", objects)
	m, _ = pressKey(m, 'l') // Enter folder/
	m, _ = updateModel(m, tui.ObjectsMsg{Bucket: "b1", Prefix: "folder/", List: &gcs.ObjectList{
		Objects: []gcs.ObjectMetadata{
			{Name: "folder/file1.txt", Bucket: "b1"},
			{Name: "folder/file2.txt", Bucket: "b1"},
		},
	}})

	// 2. Select both items
	m, _ = pressKey(m, ' ') // Select file1
	m, _ = pressKey(m, 'j') // Move to file2
	m, _ = pressKey(m, ' ') // Select file2

	// 3. Press 'x' to initiate delete
	m, _ = pressKey(m, 'x')
	assert.Assert(t, strings.Contains(stripAnsi(m.View()), "2 selected items"))

	// 4. Press 'y' to confirm delete
	_, cmd := pressKey(m, 'y')
	_ = cmd

	// 5. Process the deletion messages
	// Simulate first DeleteMsg
	m, cmd1 := updateModel(m, tui.DeleteMsg{Name: "folder/file1.txt", GoBack: false})
	// cmd1 should only be status clear
	msgs1 := resolveAllFetchCmds(cmd1)
	assert.Equal(t, len(msgs1), 0, "Should not trigger refresh yet")

	// Simulate second DeleteMsg
	m, cmd2 := updateModel(m, tui.DeleteMsg{Name: "folder/file2.txt", GoBack: false})

	// cmd2 should contain the back navigation command
	assert.Assert(t, cmd2 != nil)
	msgs2 := resolveAllFetchCmds(cmd2)
	foundBack := false
	for _, msg := range msgs2 {
		m, _ = updateModel(m, msg)
		// If m.currentPrefix changed, it worked
		if m.FullPath() == "gs://b1/folder/" {
			foundBack = true
		}
	}

	assert.Assert(t, foundBack, "Should have navigated back to parent prefix")
}

func TestModel_MultiDelete_LastErrorFails(t *testing.T) {
	projects := []gcs.ProjectBuckets{{ProjectID: "p1", Buckets: []string{"b1"}}}
	objects := simpleObjectList([]string{"obj1", "obj2"}, nil)
	for i := range objects.Objects {
		objects.Objects[i].Bucket = "b1"
	}
	m, _ := setupTestModel(projects, objects, "/tmp")

	// Enter bucket
	m = enterBucket(m, projects, "b1", objects)

	// 1. Select both items
	m, _ = pressKey(m, ' ') // Select obj1
	m, _ = pressKey(m, 'j') // Move to obj2
	m, _ = pressKey(m, ' ') // Select obj2

	// 2. Press 'x' then 'y'
	m, _ = pressKey(m, 'x')
	m, _ = pressKey(m, 'y')

	// 3. Simulate first DeleteMsg SUCCESS
	m, _ = updateModel(m, tui.DeleteMsg{Name: "obj1", GoBack: false})

	// 4. Simulate second DeleteMsg FAILURE
	_, cmd := updateModel(m, tui.DeleteMsg{Name: "obj2", GoBack: false, Err: fmt.Errorf("some error")})

	// EXPECTATION: Even if the last item failed, we should still trigger a refresh
	// because the batch is finished.
	// Currently, it returns early on error, so cmd will be a status clear but NOT include a refresh.
	msgs := resolveAllFetchCmds(cmd)
	foundRefresh := false
	for _, msg := range msgs {
		if _, ok := msg.(tui.ObjectsPageMsg); ok {
			foundRefresh = true
		}
	}
	assert.Assert(t, foundRefresh, "Should have triggered refresh even if last item failed")
}
