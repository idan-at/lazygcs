package gcs_test

import (
	"context"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"gotest.tools/v3/assert"
	"lazygcs/internal/gcs"
)

func TestClient_ListBuckets(t *testing.T) {
	server, err := fakestorage.NewServerWithOptions(fakestorage.Options{
		InitialObjects: []fakestorage.Object{
			{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName: "b1",
					Name:       "o1",
				},
			},
			{
				ObjectAttrs: fakestorage.ObjectAttrs{
					BucketName: "b2",
					Name:       "o2",
				},
			},
		},
		Host:   "127.0.0.1",
		Port:   8082,
		Scheme: "http",
	})
	assert.NilError(t, err)
	defer server.Stop()

	client := gcs.NewClient(server.Client())

	buckets, err := client.ListBuckets(context.Background(), []string{"test-project"})
	assert.NilError(t, err)

	assert.Assert(t, len(buckets) >= 2)
	assert.Assert(t, contains(buckets, "b1"))
	assert.Assert(t, contains(buckets, "b2"))
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
