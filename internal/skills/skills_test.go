package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDirs(t *testing.T) {
	home := t.TempDir()
	if dirs := DefaultDirs(home); len(dirs) != 1 || dirs[0] != filepath.Join(home, ".agents", "skills") {
		t.Fatalf("without Claude Code: %v", dirs)
	}
	_ = os.Mkdir(filepath.Join(home, ".claude"), 0o755)
	if dirs := DefaultDirs(home); len(dirs) != 2 || dirs[1] != filepath.Join(home, ".claude", "skills") {
		t.Fatalf("with Claude Code: %v", dirs)
	}
	written, err := Install([]byte("skill"), DefaultDirs(home))
	if err != nil || len(written) != 2 {
		t.Fatalf("Install: %v %v", written, err)
	}
	removed, _ := Uninstall(DefaultDirs(home))
	if len(removed) != 2 {
		t.Fatalf("Uninstall: %v", removed)
	}
}
