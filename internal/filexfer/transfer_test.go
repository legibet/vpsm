package filexfer

import (
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestTempRemotePathUsesSiblingFile(t *testing.T) {
	t.Parallel()

	target := "/srv/app/config.yaml"
	temp := tempRemotePath(target)

	if temp == target {
		t.Fatalf("expected temp path to differ from target")
	}
	if got, want := path.Dir(temp), path.Dir(target); got != want {
		t.Fatalf("expected temp file in same remote directory: got %q want %q", got, want)
	}
	if !strings.Contains(path.Base(temp), ".config.yaml.vpsm-part-") {
		t.Fatalf("unexpected temp remote filename %q", temp)
	}
}

func TestTempLocalPathUsesSiblingFile(t *testing.T) {
	t.Parallel()

	target := filepath.Join("/tmp", "archive.tar.gz")
	temp := tempLocalPath(target)

	if temp == target {
		t.Fatalf("expected temp path to differ from target")
	}
	if got, want := filepath.Dir(temp), filepath.Dir(target); got != want {
		t.Fatalf("expected temp file in same local directory: got %q want %q", got, want)
	}
	if !strings.Contains(filepath.Base(temp), ".archive.tar.gz.vpsm-part-") {
		t.Fatalf("unexpected temp local filename %q", temp)
	}
}
