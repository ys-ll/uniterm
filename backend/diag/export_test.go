package diag

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExportBundle(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, nil); err != nil {
		t.Fatal(err)
	}
	Info("test", "x", nil)
	Close()

	out := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if err := ExportBundle(dir, out); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, _ := gzip.NewReader(f)
	tr := tar.NewReader(gz)
	sawManifest := false
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Name == "manifest.json" {
			sawManifest = true
		}
	}
	if !sawManifest {
		t.Fatal("manifest.json missing from bundle")
	}
}