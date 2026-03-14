package preview

import "testing"

func TestDockerArchivePreviewer_CanPreview(t *testing.T) {
	p := &DockerArchivePreviewer{}

	tests := []struct {
		name     string
		obj      Object
		expected bool
	}{
		{"tar", Object{Name: "image.tar"}, true},
		{"tgz", Object{Name: "image.tgz"}, true},
		{"tar.gz", Object{Name: "image.tar.gz"}, true},
		{"txt", Object{Name: "image.txt"}, false},
		{"content-type tar", Object{Name: "image", ContentType: "application/x-tar"}, true},
		{"content-type gzip", Object{Name: "image", ContentType: "application/gzip"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.CanPreview(tt.obj)
			if result != tt.expected {
				t.Errorf("CanPreview(%+v) = %v; want %v", tt.obj, result, tt.expected)
			}
		})
	}
}
