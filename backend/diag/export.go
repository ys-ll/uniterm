package diag

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

type BundleManifest struct {
	AppVersion string   `json:"appVersion"`
	BuiltAt    string   `json:"builtAt"`
	Runtime    string   `json:"runtime"`
	GoVersion  string   `json:"goVersion"`
	GOOS       string   `json:"goos"`
	GOARCH     string   `json:"goarch"`
	NumCPU     int      `json:"numCPU"`
	LogFiles   []string `json:"logFiles"`
}

func ExportBundle(dir, targetPath string) error {
	f, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	manifest := BundleManifest{
		Runtime:   runtime.Version(),
		GOOS:      runtime.GOOS,
		GOARCH:    runtime.GOARCH,
		NumCPU:    runtime.NumCPU(),
		BuiltAt:   time.Now().UTC().Format(time.RFC3339Nano),
		LogFiles:  []string{},
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !hasLogSuffix(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			continue
		}
		hdr.Name = "logs/" + info.Name()
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		fp := filepath.Join(dir, info.Name())
		src, err := os.Open(fp)
		if err != nil {
			continue
		}
		if _, err := io.Copy(tw, src); err != nil {
			src.Close()
			return err
		}
		src.Close()
		manifest.LogFiles = append(manifest.LogFiles, hdr.Name)
	}
	mb, _ := json.MarshalIndent(manifest, "", "  ")
	body := append(mb, '\n')
	if err := tw.WriteHeader(&tar.Header{
		Name:    "manifest.json",
		Mode:    0o644,
		Size:    int64(len(body)),
		ModTime: time.Now(),
		Typeflag: tar.TypeReg,
	}); err != nil {
		return err
	}
	if _, err := tw.Write(body); err != nil {
		return err
	}
	return nil
}