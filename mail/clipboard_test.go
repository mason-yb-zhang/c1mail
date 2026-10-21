package mail

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClipboardRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clip.txt")
	t.Setenv("C1CLIPBOARD", path)
	if err := WriteClipboard("你好\nworld"); err != nil {
		t.Fatal(err)
	}
	got := ReadClipboard()
	if got != "你好\nworld" {
		t.Fatalf("got %q", got)
	}
	// 空文件
	os.WriteFile(path, []byte(""), 0o644)
	if ReadClipboard() != "" {
		t.Fatal("want empty")
	}
}
