package diag

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Rotator handles file-size and directory-size based log rotation. The
// default is 10 MiB per file, 50 MiB total, keep 5 files.
type Rotator struct {
	Dir         string
	Keep        int
	FileSizeCap int64
	DirSizeCap  int64
}

// MaybeRotate renames the current log to .1, .2, .. .N, dropping files
// outside Keep. No-op if currentSize is below the file cap.
func (r *Rotator) MaybeRotate(currentSize int64) error {
	if currentSize < r.FileSizeCap {
		return nil
	}
	for i := r.Keep - 1; i >= 1; i-- {
		older := filepath.Join(r.Dir, fmt.Sprintf("uniterm.log.%d", i))
		newer := filepath.Join(r.Dir, fmt.Sprintf("uniterm.log.%d", i+1))
		if _, err := os.Stat(older); err == nil {
			_ = os.Rename(older, newer)
		}
	}
	cur := filepath.Join(r.Dir, "uniterm.log")
	if _, err := os.Stat(cur); err == nil {
		_ = os.Rename(cur, filepath.Join(r.Dir, "uniterm.log.1"))
	}
	return r.EnforceDirCap()
}

// EnforceDirCap removes the oldest log files until the directory sum
// is at or below DirSizeCap.
func (r *Rotator) EnforceDirCap() error {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return err
	}
	type fi struct {
		name  string
		size  int64
		mtime int64
	}
	var infos []fi
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Treat any file containing ".log" as a log file (current uniterm.log
		// or rotated variants uniterm.log.1, uniterm.log.x, etc.).
		if !isLogFile(info.Name()) {
			continue
		}
		infos = append(infos, fi{info.Name(), info.Size(), info.ModTime().UnixNano()})
		total += info.Size()
	}
	if total <= r.DirSizeCap {
		return nil
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].mtime < infos[j].mtime })
	for _, f := range infos {
		if total <= r.DirSizeCap {
			break
		}
		if err := os.Remove(filepath.Join(r.Dir, f.name)); err == nil {
			total -= f.size
		}
	}
	return nil
}

// hasLogSuffix reports whether name ends with ".log".
func hasLogSuffix(name string) bool {
	return len(name) >= 4 && name[len(name)-4:] == ".log"
}

// isLogFile accepts any file with ".log" in the name, so rotated
// variants like "uniterm.log.1" are recognized too.
func isLogFile(name string) bool {
	return len(name) >= 4 && (hasLogSuffix(name) || contains(name, ".log"))
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// configureRotator is the rotater hook called by Init. It must be
// called while mu is held (Init does so).
func configureRotator(dir string, cfg *DiagConfig) {
	if cfg == nil {
		cfg = defaultConfig()
	}
	rotator = &Rotator{
		Dir:         dir,
		Keep:        cfg.KeepFiles,
		FileSizeCap: cfg.FileSizeCap,
		DirSizeCap:  cfg.DirSizeCap,
	}
}

// rotateIfNeeded rotates the current log file when its size exceeds the
// cap, then re-opens a fresh writer on the new file.
func rotateIfNeeded(l *Logger) {
	if rotator == nil {
		return
	}
	info, err := l.file.Stat()
	if err != nil || info.Size() <= rotator.FileSizeCap {
		return
	}
	_ = l.bw.Flush()
	if err := rotator.MaybeRotate(info.Size()); err != nil {
		return
	}
	newF, err := os.OpenFile(filepath.Join(rotator.Dir, "uniterm.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_ = l.file.Close()
	l.file = newF
	l.bw = bufio.NewWriterSize(newF, 64*1024)
}
