package static

import "testing"

func TestEmbedFS(t *testing.T) {
	_, err := FS.ReadFile("index.html")
	if err != nil {
		t.Errorf("failed to read index.html: %v", err)
	}
}
