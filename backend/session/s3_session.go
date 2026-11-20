package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rhnvrm/simples3"
)

const (
	// Files smaller than this upload via a single PUT; multipart
	// bookkeeping (init/parts/complete) would only add round-trips.
	s3MultipartThreshold = 16 * 1024 * 1024
	// Concurrent workers for multipart parts, directory transfers and
	// batch server-side copies.
	s3TransferConcurrency = 4
	// Retries per multipart part with exponential backoff (100 ms base).
	s3PartRetries = 3
	// Progress is reported per completed part, so small parts mean smooth
	// progress. 5 MB is the S3 protocol floor (simples3.MinPartSize); the
	// size grows adaptively only when a huge upload would exceed the
	// 10000-part limit (headroom at 9000).
	s3MinPartSize = simples3.MinPartSize
	s3MaxPartSize = 5 * 1024 * 1024 * 1024
	s3MaxParts    = 9000
	// Single-file downloads at or above s3MultipartThreshold are fetched as
	// concurrent ranged GETs, mirroring multipart upload. Chunks are at least
	// 16 MB so per-request overhead stays sane, growing only so a huge object
	// stays under 1000 range requests.
	s3DownloadChunkFloor = 16 * 1024 * 1024
	s3DownloadMaxChunks  = 1000
	// Read buffer per download worker.
	s3DownloadBufSize = 128 * 1024
)

// s3DownloadChunkSizeFor picks the range chunk size for a parallel download:
// 16 MB wherever possible, growing only for objects whose chunk count would
// otherwise exceed the request budget.
func s3DownloadChunkSizeFor(size int64) int64 {
	chunk := (size + s3DownloadMaxChunks - 1) / s3DownloadMaxChunks
	if chunk < s3DownloadChunkFloor {
		chunk = s3DownloadChunkFloor
	}
	return chunk
}

// s3PartSizeFor picks the multipart part size for an upload: 5 MB wherever
// possible so progress advances per 5 MB, growing only for uploads whose
// part count would otherwise exceed the protocol limit.
func s3PartSizeFor(size int64) int64 {
	partSize := (size + s3MaxParts - 1) / s3MaxParts
	if partSize < s3MinPartSize {
		partSize = s3MinPartSize
	}
	if partSize > s3MaxPartSize {
		partSize = s3MaxPartSize
	}
	return partSize
}

type S3Session struct {
	baseSession
	localFSOps
	s3        *simples3.S3
	client    *http.Client
	bucket    string
	cwd       string
	mu        sync.RWMutex
	transfers map[string]*TransferTask
	taskSeq   int64
}

func NewS3Session(id string) *S3Session {
	return &S3Session{
		baseSession: baseSession{
			id:          id,
			sessionType: "s3",
			status:      StatusDisconnected,
		},
		localFSOps: newLocalFSOps(),
		cwd:        "/",
		transfers:  make(map[string]*TransferTask),
	}
}

func (s *S3Session) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)
	if config.S3Bucket != "" {
		s.title = fmt.Sprintf("s3://%s", config.S3Bucket)
	} else {
		s.title = config.Host
	}

	s3Client := simples3.New(config.S3Region, config.User, config.Password)
	s3Client.Endpoint = strings.TrimSuffix(config.Host, "/")

	// One shared HTTP client for all S3 traffic: the default transport only
	// keeps 2 idle connections per host, which with concurrent workers meant
	// a TCP+TLS handshake per request. 16 idle slots cover the transfer
	// concurrency with headroom.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 16
	// Route through the configured proxy; nil keeps the system-default
	// ProxyFromEnvironment behavior of the cloned transport.
	if config.Proxy != nil {
		transport.Proxy = ProxyHTTPTransportProxy(config.Proxy)
	}
	s.client = &http.Client{Transport: transport}
	s3Client.SetClient(s.client)
	// S3URLStyle="" or "virtual" uses virtual-hosted URLs
	// (https://bucket.endpoint/key) — required by Alibaba Cloud OSS, Tencent
	// COS and Huawei OBS, which reject path-style requests with
	// "SecondLevelDomainForbidden" (issue #452). "path" keeps the legacy
	// path-style (https://endpoint/bucket/key) for AWS S3 and MinIO.
	if config.S3URLStyle != "path" {
		s3Client.SetVirtualHostedStyle(true)
	}

	s.s3 = s3Client
	s.bucket = config.S3Bucket
	s.cwd = "/"
	s.setStatus(StatusConnected)
	return nil
}

func (s *S3Session) Write(data []byte) error  { return nil }
func (s *S3Session) Resize(cols, rows int) error { return nil }

func (s *S3Session) Disconnect() error {
	s.s3 = nil
	if s.client != nil {
		s.client.CloseIdleConnections()
		s.client = nil
	}
	s.bucket = ""
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *S3Session) IsConnected() bool {
	return s.Status() == StatusConnected && s.s3 != nil
}

func (s *S3Session) requireClient() error {
	if s.s3 == nil {
		return fmt.Errorf("S3 session not connected")
	}
	return nil
}

func (s *S3Session) resolveRemote(p string) (string, error) {
	if p == "" {
		return s.cwd, nil
	}
	if path.IsAbs(p) {
		return p, nil
	}
	return path.Join(s.cwd, p), nil
}

func (s *S3Session) s3Key(p string) string {
	// S3 keys don't start with "/", they're relative to the bucket root.
	p = strings.TrimPrefix(p, "/")
	// Bucket root: path equals the bucket name → empty key
	if s.bucket != "" && p == s.bucket {
		return ""
	}
	// Strip bucket name prefix from subdirectory paths (e.g. "mybucket/dir" → "dir")
	if s.bucket != "" {
		bucketPrefix := s.bucket + "/"
		p = strings.TrimPrefix(p, bucketPrefix)
	}
	return p
}

func (s *S3Session) nextTaskID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddInt64(&s.taskSeq, 1))
}

func (s *S3Session) ListRemote(dir string) (FileListResult, error) {
	if err := s.requireClient(); err != nil {
		return FileListResult{}, err
	}

	// If no bucket specified, list all buckets
	if s.bucket == "" {
		resp, err := s.s3.ListBuckets(simples3.ListBucketsInput{})
		if err != nil {
			return FileListResult{}, err
		}
		files := make([]FileItem, 0, len(resp.Buckets))
		for _, b := range resp.Buckets {
			files = append(files, FileItem{
				Name:    b.Name,
				Size:    0,
				ModTime: b.CreationDate.Format("2006-01-02T15:04:05Z"),
				Mode:    "drwxr-xr-x",
				IsDir:   true,
			})
		}
		return FileListResult{Files: files, Dir: "/"}, nil
	}

	target, err := s.resolveRemote(dir)
	if err != nil {
		return FileListResult{}, err
	}

	prefix := s.s3Key(target)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	objects, commonPrefixes, err := s.listAllPaged(prefix, "/")
	if err != nil {
		return FileListResult{}, err
	}

	files := make([]FileItem, 0, len(objects)+len(commonPrefixes))

	// Process common prefixes (directories)
	for _, pfx := range commonPrefixes {
		name := strings.TrimPrefix(pfx, prefix)
		name = strings.TrimSuffix(name, "/")
		if name == "" {
			continue
		}
		files = append(files, FileItem{
			Name:    name,
			Size:    0,
			ModTime: "",
			Mode:    "drwxr-xr-x",
			IsDir:   true,
		})
	}

	// Process objects (files and directory markers)
	for _, obj := range objects {
		name := strings.TrimPrefix(obj.Key, prefix)
		if name == "" || strings.HasSuffix(name, "/") {
			// Skip the directory itself or directory marker objects
			continue
		}
		files = append(files, FileItem{
			Name:    name,
			Size:    obj.Size,
			ModTime: obj.LastModified,
			Mode:    "-rw-r--r--",
			IsDir:   false,
		})
	}

	return FileListResult{Files: files, Dir: target}, nil
}

func (s *S3Session) ChangeRemoteDir(dir string) (FileListResult, error) {
	if err := s.requireClient(); err != nil {
		return FileListResult{}, err
	}
	target, err := s.resolveRemote(dir)
	if err != nil {
		return FileListResult{}, err
	}

	// ── Bucket list level ──
	if s.bucket == "" {
		if target == "/" {
			return s.ListRemote("/") // refresh bucket list
		}
		// Enter a bucket
		bucketName := strings.TrimPrefix(target, "/")
		s.mu.Lock()
		s.bucket = bucketName
		s.cwd = "/" + bucketName
		s.mu.Unlock()
		return s.ListRemote("/" + bucketName)
	}

	// ── Inside a bucket ──
	bucketRoot := "/" + s.bucket

	// "/" inside a bucket means the bucket list root
	if target == "/" {
		s.mu.Lock()
		s.bucket = ""
		s.cwd = "/"
		s.mu.Unlock()
		return s.ListRemote("/")
	}

	// Navigate to bucket root via breadcrumb or direct path
	if target == bucketRoot {
		s.mu.Lock()
		s.cwd = bucketRoot
		s.mu.Unlock()
		return s.ListRemote(bucketRoot)
	}

	// ".." navigation within the bucket
	if dir == ".." {
		parent := path.Dir(s.cwd)
		s.mu.Lock()
		s.cwd = parent
		s.mu.Unlock()
		return s.ListRemote(parent)
	}

	// Validate directory exists by listing it
	prefix := s.s3Key(target)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	input := simples3.ListInput{
		Bucket:    s.bucket,
		Prefix:    prefix,
		Delimiter: "/",
		MaxKeys:   1,
	}
	resp, err := s.s3.List(input)
	if err != nil {
		return FileListResult{}, fmt.Errorf("no such directory: %s", target)
	}
	if len(resp.Objects) == 0 && len(resp.CommonPrefixes) == 0 && prefix != "" {
		// Could be a "virtual" directory (no marker object). Allow it.
	}

	s.mu.Lock()
	s.cwd = target
	s.mu.Unlock()
	return s.ListRemote(target)
}

// Symlink is not supported: object storage has no links.
func (s *S3Session) Symlink(_, _ string) error {
	return fmt.Errorf("symlink is not supported by S3")
}

func (s *S3Session) MakeDir(dir string) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	if s.bucket == "" {
		return fmt.Errorf("cannot create a directory at the S3 root: open a bucket first")
	}
	p, err := s.resolveRemote(dir)
	if err != nil {
		return err
	}
	key := s.s3Key(p)
	if key != "" && !strings.HasSuffix(key, "/") {
		key += "/"
	}
	// Create an empty directory marker object
	input := simples3.UploadInput{
		Bucket:      s.bucket,
		ObjectKey:   key,
		ContentType: "application/x-directory",
		Body:        bytes.NewReader([]byte{}),
	}
	_, err = s.s3.FilePut(input)
	return err
}

func (s *S3Session) Remove(p string, recursive bool) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	target, err := s.resolveRemote(p)
	if err != nil {
		return err
	}

	key := s.s3Key(target)

	if recursive {
		// List all objects with this prefix (paginated) and delete them in
		// batches of up to 1000 keys per DeleteObjects request. S3 treats
		// already-absent keys as successfully deleted, so the prefix and
		// directory marker objects can join the same batch.
		prefix := key
		if prefix != "" && !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		objects, _, err := s.listAllPaged(prefix, "")
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(objects)+2)
		for _, obj := range objects {
			keys = append(keys, obj.Key)
		}
		// The prefix and directory marker objects can join the same batch:
		// S3 treats already-absent keys as successfully deleted.
		if key != "" {
			keys = append(keys, key, key+"/")
		}
		return s.deleteBatch(keys)
	}

	// Non-recursive: try file first, then directory marker
	err = s.s3.FileDelete(simples3.DeleteInput{
		Bucket:    s.bucket,
		ObjectKey: key,
	})
	if err == nil {
		return nil
	}

	// Try with trailing slash (directory marker)
	if key != "" {
		err2 := s.s3.FileDelete(simples3.DeleteInput{
			Bucket:    s.bucket,
			ObjectKey: key + "/",
		})
		if err2 == nil {
			// Check if directory with this prefix has any contents
			prefix := key + "/"
			input := simples3.ListInput{
				Bucket:    s.bucket,
				Prefix:    prefix,
				Delimiter: "/",
				MaxKeys:   1,
			}
			resp, _ := s.s3.List(input)
			if len(resp.Objects) > 0 || len(resp.CommonPrefixes) > 0 {
				// Re-create the directory marker since it had contents
				s.s3.FilePut(simples3.UploadInput{
					Bucket:      s.bucket,
					ObjectKey:   prefix,
					ContentType: "application/x-directory",
					Body:        bytes.NewReader([]byte{}),
				})
				return fmt.Errorf("directory not empty")
			}
			return nil
		}
	}
	return err
}

func (s *S3Session) Rename(oldName, newName string) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	old, err := s.resolveRemote(oldName)
	if err != nil {
		return err
	}
	n, err := s.resolveRemote(newName)
	if err != nil {
		return err
	}

	oldKey := s.s3Key(old)
	newKey := s.s3Key(n)

	// Check if source is a directory
	isDir := false
	prefix := oldKey
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	input := simples3.ListInput{
		Bucket:    s.bucket,
		Prefix:    prefix,
		MaxKeys:   1,
	}
	resp, err := s.s3.List(input)
	if err != nil {
		return err
	}
	if len(resp.Objects) > 0 {
		// Has contents, treat as directory
		isDir = true
	}

	if isDir {
		// Rename all objects with this prefix: concurrent server-side copies,
		// then a batched delete of the sources once every copy succeeded.
		listResp, _, err := s.listAllPaged(prefix, "")
		if err != nil {
			return err
		}
		newPrefix := newKey
		if newPrefix != "" && !strings.HasSuffix(newPrefix, "/") {
			newPrefix += "/"
		}
		type mvItem struct{ src, dst string }
		items := make([]mvItem, 0, len(listResp))
		for _, obj := range listResp {
			suffix := strings.TrimPrefix(obj.Key, oldKey)
			if strings.HasPrefix(suffix, "/") {
				suffix = strings.TrimPrefix(suffix, "/")
			}
			items = append(items, mvItem{obj.Key, newPrefix + suffix})
		}
		if err := runPool(context.Background(), len(items), s3TransferConcurrency, func(i int) error {
			_, err := s.s3.CopyObject(simples3.CopyObjectInput{
				SourceBucket: s.bucket,
				SourceKey:    items[i].src,
				DestBucket:   s.bucket,
				DestKey:      items[i].dst,
			})
			return err
		}); err != nil {
			return err
		}
		srcKeys := make([]string, len(items))
		for i, it := range items {
			srcKeys[i] = it.src
		}
		return s.deleteBatch(srcKeys)
	}

	// File rename: copy + delete
	if _, err := s.s3.CopyObject(simples3.CopyObjectInput{
		SourceBucket: s.bucket,
		SourceKey:    oldKey,
		DestBucket:   s.bucket,
		DestKey:      newKey,
	}); err != nil {
		return err
	}
	return s.s3.FileDelete(simples3.DeleteInput{
		Bucket:    s.bucket,
		ObjectKey: oldKey,
	})
}

func (s *S3Session) Chmod(p string, mode os.FileMode) error {
	return fmt.Errorf("S3 does not support chmod")
}

func (s *S3Session) Copy(oldPath, newPath string) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	old, err := s.resolveRemote(oldPath)
	if err != nil {
		return err
	}
	n, err := s.resolveRemote(newPath)
	if err != nil {
		return err
	}

	oldKey := s.s3Key(old)
	newKey := s.s3Key(n)

	// Check if source is a directory
	isDir := false
	prefix := oldKey
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	input := simples3.ListInput{
		Bucket:    s.bucket,
		Prefix:    prefix,
		MaxKeys:   1,
	}
	resp, err := s.s3.List(input)
	if err != nil {
		return err
	}
	if len(resp.Objects) > 0 {
		isDir = true
	}

	if isDir {
		// Copy all objects with this prefix, concurrently server-side.
		listResp, _, err := s.listAllPaged(prefix, "")
		if err != nil {
			return err
		}
		newPrefix := newKey
		if newPrefix != "" && !strings.HasSuffix(newPrefix, "/") {
			newPrefix += "/"
		}
		type cpItem struct{ src, dst string }
		items := make([]cpItem, 0, len(listResp))
		for _, obj := range listResp {
			suffix := strings.TrimPrefix(obj.Key, oldKey)
			if strings.HasPrefix(suffix, "/") {
				suffix = strings.TrimPrefix(suffix, "/")
			}
			items = append(items, cpItem{obj.Key, newPrefix + suffix})
		}
		return runPool(context.Background(), len(items), s3TransferConcurrency, func(i int) error {
			_, err := s.s3.CopyObject(simples3.CopyObjectInput{
				SourceBucket: s.bucket,
				SourceKey:    items[i].src,
				DestBucket:   s.bucket,
				DestKey:      items[i].dst,
			})
			return err
		})
	}

	_, err = s.s3.CopyObject(simples3.CopyObjectInput{
		SourceBucket: s.bucket,
		SourceKey:    oldKey,
		DestBucket:   s.bucket,
		DestKey:      newKey,
	})
	return err
}

func (s *S3Session) Move(oldPath, newPath string) error {
	return s.Rename(oldPath, newPath)
}

func (s *S3Session) GetContent(remotePath string) ([]byte, error) {
	if err := s.requireClient(); err != nil {
		return nil, err
	}
	p, err := s.resolveRemote(remotePath)
	if err != nil {
		return nil, err
	}
	rc, err := s.s3.FileDownload(simples3.DownloadInput{
		Bucket:    s.bucket,
		ObjectKey: s.s3Key(p),
	})
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func (s *S3Session) PutContent(remotePath string, content []byte) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	if s.bucket == "" {
		return fmt.Errorf("cannot create a file at the S3 root: open a bucket first")
	}
	p, err := s.resolveRemote(remotePath)
	if err != nil {
		return err
	}
	input := simples3.UploadInput{
		Bucket:      s.bucket,
		ObjectKey:   s.s3Key(p),
		ContentType: "application/octet-stream",
		Body:        bytes.NewReader(content),
	}
	_, err = s.s3.FilePut(input)
	return err
}

func (s *S3Session) Get(remotePath, localPath string, recursive bool) (string, error) {
	if err := s.requireClient(); err != nil {
		return "", err
	}
	rp, err := s.resolveRemote(remotePath)
	if err != nil {
		return "", err
	}
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	task := &TransferTask{
		ID:         s.nextTaskID("dl"),
		Type:       "download",
		LocalPath:  lp,
		RemotePath: rp,
		Status:     "running",
	}
	task.start()
	s.mu.Lock()
	s.transfers[task.ID] = task
	s.mu.Unlock()
	s.emitTransferStart(task)
	go func() {
		defer func() {
			task.done()
			if task.Status == "error" {
				// Retained in s.transfers for retry; the frontend drops it
				// via DismissTransfer.
				return
			}
			s.mu.Lock()
			delete(s.transfers, task.ID)
			s.mu.Unlock()
		}()
		var err error
		if recursive {
			err = s.downloadDir(rp, lp, task)
		} else {
			err = s.downloadFile(task, rp, lp)
		}
		if err != nil {
			if task.ctx.Err() != nil {
				// cancelled mid-transfer: not a transfer error
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			s.emitTransferEvent(task, err)
			return
		}
		task.Status = "done"
		s.emitTransferComplete(task)
	}()
	return task.ID, nil
}

func (s *S3Session) Put(localPath, remotePath string, recursive bool) (string, error) {
	if err := s.requireClient(); err != nil {
		return "", err
	}
	if s.bucket == "" {
		return "", fmt.Errorf("cannot upload at the S3 root: open a bucket first")
	}
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	rp, err := s.resolveRemote(remotePath)
	if err != nil {
		return "", err
	}
	task := &TransferTask{
		ID:         s.nextTaskID("ul"),
		Type:       "upload",
		LocalPath:  lp,
		RemotePath: rp,
		Status:     "running",
	}
	task.start()
	s.mu.Lock()
	s.transfers[task.ID] = task
	s.mu.Unlock()
	s.emitTransferStart(task)
	go func() {
		defer func() {
			task.done()
			if task.Status == "error" {
				// Retained in s.transfers for retry; the frontend drops it
				// via DismissTransfer.
				return
			}
			s.mu.Lock()
			delete(s.transfers, task.ID)
			s.mu.Unlock()
		}()
		var err error
		if recursive {
			err = s.uploadDir(lp, rp, task)
		} else {
			err = s.uploadFile(task, lp, rp)
		}
		if err != nil {
			if task.ctx.Err() != nil {
				// cancelled mid-transfer: not a transfer error
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			s.emitTransferEvent(task, err)
			return
		}
		task.Status = "done"
		s.emitTransferComplete(task)
	}()
	return task.ID, nil
}

func (s *S3Session) CancelTransfer(taskID string) error {
	s.mu.Lock()
	task, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if task.cancel != nil {
		task.cancel()
	}
	return nil
}

// RetryTransfer (re)starts a transfer from a frontend-held checkpoint. For
// recursive transfers, files listed in skipCompleted (paths relative to the
// transfer root, '/'-separated) are counted as done without re-transferring.
// The task is re-created directly (not via Get/Put) because the spec carries
// the already-resolved remote path from the original task.
func (s *S3Session) RetryTransfer(spec TransferSpec, skipCompleted []string) (string, error) {
	if err := s.requireClient(); err != nil {
		return "", err
	}
	tfType := spec.Type
	prefix := "dl"
	if tfType != "download" {
		tfType = "upload"
		prefix = "ul"
	}
	task := &TransferTask{
		ID:         s.nextTaskID(prefix),
		Type:       tfType,
		LocalPath:  spec.LocalPath,
		RemotePath: spec.RemotePath,
		Status:     "running",
	}
	if spec.Recursive && len(skipCompleted) > 0 {
		task.SetSkip(skipCompleted)
	}
	task.start()
	s.mu.Lock()
	s.transfers[task.ID] = task
	s.mu.Unlock()
	s.emitTransferStart(task)
	go func() {
		defer func() {
			task.done()
			if task.Status == "error" {
				return // retained for retry
			}
			s.mu.Lock()
			delete(s.transfers, task.ID)
			s.mu.Unlock()
		}()
		var err error
		if spec.Recursive {
			if tfType == "download" {
				err = s.downloadDir(spec.RemotePath, spec.LocalPath, task)
			} else {
				err = s.uploadDir(spec.LocalPath, spec.RemotePath, task)
			}
		} else {
			if tfType == "download" {
				err = s.downloadFile(task, spec.RemotePath, spec.LocalPath)
			} else {
				err = s.uploadFile(task, spec.LocalPath, spec.RemotePath)
			}
		}
		if err != nil {
			if task.ctx.Err() != nil {
				// cancelled mid-transfer: not a transfer error
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			s.emitTransferEvent(task, err)
			return
		}
		task.Status = "done"
		s.emitTransferComplete(task)
	}()
	return task.ID, nil
}

// DismissTransfer drops a retained (failed) task from the transfers map.
func (s *S3Session) DismissTransfer(taskID string) error {
	s.mu.Lock()
	delete(s.transfers, taskID)
	s.mu.Unlock()
	return nil
}

func (s *S3Session) PauseTransfer(taskID string) error {
	s.mu.Lock()
	task, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	task.setPaused(true)
	task.Status = "paused"
	s.emitTransferPaused(task)
	return nil
}

func (s *S3Session) ResumeTransfer(taskID string) error {
	s.mu.Lock()
	task, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	task.setPaused(false)
	task.Status = "running"
	close(task.pauseCh)
	task.pauseCh = make(chan struct{})
	s.emitTransferResumed(task)
	return nil
}

// listAllPaged lists every object and common prefix under a prefix, following
// continuation tokens. S3 caps each response at 1000 keys; without pagination
// large trees were silently truncated.
func (s *S3Session) listAllPaged(prefix, delimiter string) ([]simples3.Object, []string, error) {
	var objects []simples3.Object
	var prefixes []string
	token := ""
	for {
		input := simples3.ListInput{
			Bucket:    s.bucket,
			Prefix:    prefix,
			Delimiter: delimiter,
		}
		if token != "" {
			input.ContinuationToken = token
		}
		resp, err := s.s3.List(input)
		if err != nil {
			return nil, nil, err
		}
		objects = append(objects, resp.Objects...)
		prefixes = append(prefixes, resp.CommonPrefixes...)
		if !resp.IsTruncated || resp.NextContinuationToken == "" {
			return objects, prefixes, nil
		}
		token = resp.NextContinuationToken
	}
}

// deleteBatch removes keys with DeleteObjects requests, up to 1000 keys each,
// instead of one request per object. S3 treats absent keys as deleted.
func (s *S3Session) deleteBatch(keys []string) error {
	const batchSize = 1000
	for start := 0; start < len(keys); start += batchSize {
		end := start + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		if _, err := s.s3.DeleteObjects(simples3.DeleteObjectsInput{
			Bucket:  s.bucket,
			Objects: keys[start:end],
			Quiet:   true,
		}); err != nil {
			return err
		}
	}
	return nil
}

// runPool runs fn(i) for i in [0, count) across up to workers goroutines.
// Workers stop early when ctx is cancelled or fn reports an error; the first
// error (or ctx.Err()) is returned. Callers that want per-item failures to
// not abort the batch handle them inside fn and return nil.
func runPool(ctx context.Context, count, workers int, fn func(i int) error) error {
	if count <= 0 {
		return nil
	}
	if workers > count {
		workers = count
	}
	if workers < 1 {
		workers = 1
	}
	idx := make(chan int)
	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range idx {
				if ctx.Err() != nil {
					return
				}
				if err := fn(i); err != nil {
					select {
					case errCh <- err:
					default:
					}
					return
				}
			}
		}()
	}
feed:
	for i := 0; i < count; i++ {
		select {
		case idx <- i:
		case err := <-errCh:
			// Buffer the error back and stop feeding; workers drain idx.
			errCh <- err
			break feed
		case <-ctx.Done():
			break feed
		}
	}
	close(idx)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

// s3ContentType picks a content type from a file extension.
func s3ContentType(ext string) string {
	switch strings.ToLower(ext) {
	case ".txt":
		return "text/plain"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

func (s *S3Session) downloadDir(remoteDir, localDir string, task *TransferTask) error {
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return err
	}

	prefix := s.s3Key(remoteDir)
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	// One flat paginated listing serves both the progress total and the work
	// list; a delimiter walk would only see the first 1000 keys per level.
	objects, _, err := s.listAllPaged(prefix, "")
	if err != nil {
		return err
	}
	type dlItem struct {
		key, rel string
		size     int64
	}
	items := make([]dlItem, 0, len(objects))
	var total int64
	for _, obj := range objects {
		rel := strings.TrimPrefix(obj.Key, prefix)
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue // directory markers
		}
		items = append(items, dlItem{obj.Key, rel, obj.Size})
		total += obj.Size
	}
	if task.loadTotal() <= 0 {
		task.setTotal(total)
	}
	task.setFileCount(len(items))

	return runPool(task.ctx, len(items), s3TransferConcurrency, func(i int) error {
		item := items[i]
		if task.shouldSkip(item.rel) {
			// Retry checkpoint: transferred in a previous attempt.
			task.addProgress(item.size)
			task.beginFile(item.rel)
			task.finishFile(item.rel)
			return nil
		}
		lp := filepath.Join(localDir, filepath.FromSlash(item.rel))
		task.beginFile(item.rel)
		s.emitFileStart(task, item.rel, filepath.Base(lp))
		if err := s.downloadObject(task, item.key, lp, item.size); err != nil {
			if task.ctx.Err() != nil {
				return err // cancelled: stop the whole transfer
			}
			task.failFile(item.rel, err)
			s.emitFileFailed(task, item.rel, err)
			return nil
		}
		task.finishFile(item.rel)
		s.emitFileDone(task, item.rel)
		return nil
	})
}

func (s *S3Session) downloadFile(task *TransferTask, remotePath, localPath string) error {
	key := s.s3Key(remotePath)
	// Get file size first: progress total, and the switch between parallel
	// ranged download and single-stream for small objects.
	var size int64 = -1
	if task.loadTotal() <= 0 {
		details, err := s.s3.FileDetails(simples3.DetailsInput{
			Bucket:    s.bucket,
			ObjectKey: key,
		})
		if err == nil && details.ContentLength != "" {
			if v, parseErr := strconv.ParseInt(details.ContentLength, 10, 64); parseErr == nil {
				size = v
				task.setTotal(size)
			}
		}
	}
	return s.downloadObject(task, key, localPath, size)
}

// downloadObject streams one object to disk by its raw S3 key. Objects at or
// above s3MultipartThreshold go through concurrent ranged GETs; anything
// smaller (or an endpoint without range support) uses a single stream.
func (s *S3Session) downloadObject(task *TransferTask, key, localPath string, size int64) error {
	if size >= s3MultipartThreshold {
		err := s.s3DownloadRanged(task, key, localPath, size)
		if !errors.Is(err, errS3RangeUnsupported) {
			return err
		}
		// Endpoint ignored the Range (answered 200) or rejected the request:
		// fall back to a single-stream download.
	}
	return s.downloadObjectSingle(task, key, localPath)
}

// downloadObjectSingle streams one object with a single GET, one connection.
func (s *S3Session) downloadObjectSingle(task *TransferTask, key, localPath string) error {
	rc, err := s.s3.FileDownload(simples3.DownloadInput{
		Bucket:    s.bucket,
		ObjectKey: key,
	})
	if err != nil {
		return err
	}
	defer rc.Close()

	dst, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	buf := make([]byte, 64*1024)
	for {
		select {
		case <-task.ctx.Done():
			return task.ctx.Err()
		default:
		}
		task.waitIfPaused()
		n, e := rc.Read(buf)
		if n > 0 {
			dst.Write(buf[:n])
			task.addProgress(int64(n))
			s.emitTransferProgress(task)
		}
		if e != nil {
			if e == io.EOF {
				return nil
			}
			return e
		}
	}
}

// errS3RangeUnsupported marks an endpoint where ranged GET is unavailable —
// the Range header ignored (200 instead of 206) or the request rejected with
// a 4xx (e.g. a provider that won't take path-style presigned URLs). The
// caller falls back to a single-stream download for the whole object.
var errS3RangeUnsupported = errors.New("s3: ranged download unsupported")

// s3RangeStatusError reports a ranged GET answered with an unexpected status.
// Kept typed so the chunk worker can tell a hard "no range support" (200/4xx)
// from a transient 5xx worth retrying.
type s3RangeStatusError struct {
	code   int
	status string
	body   string
}

func (e *s3RangeStatusError) Error() string {
	if e.body != "" {
		return fmt.Sprintf("status code: %s: %s", e.status, e.body)
	}
	return fmt.Sprintf("status code: %s", e.status)
}

// s3DownloadRanged fetches a large object as concurrent ranged GETs. The file
// is preallocated and each worker streams its chunks straight to their own
// byte offsets via WriteAt, so memory stays flat and workers never contend.
// A chunk that comes back short is retried for just its missing remainder.
func (s *S3Session) s3DownloadRanged(task *TransferTask, key, localPath string, size int64) error {
	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(size); err != nil {
		return err
	}

	chunkSize := s3DownloadChunkSizeFor(size)
	totalChunks := int((size + chunkSize - 1) / chunkSize)

	err = runPool(task.ctx, totalChunks, s3TransferConcurrency, func(i int) error {
		start := int64(i) * chunkSize
		end := start + chunkSize
		if end > size {
			end = size
		}
		written := int64(0)
		for attempt := 0; attempt <= s3PartRetries; attempt++ {
			if attempt > 0 {
				select {
				case <-time.After(time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond):
				case <-task.ctx.Done():
					return task.ctx.Err()
				}
			}
			task.waitIfPaused()
			rc, err := s.rangeGet(key, start+written, end)
			if err != nil {
				var se *s3RangeStatusError
				if errors.As(err, &se) && (se.code == http.StatusOK || (se.code >= 400 && se.code < 500)) {
					return errS3RangeUnsupported
				}
				continue // transient: retry the remaining range
			}
			got, readErr := s.readRangeInto(task, rc, f, start+written)
			written += got
			if readErr == nil && written == end-start {
				return nil
			}
			if errors.Is(readErr, task.ctx.Err()) {
				return readErr
			}
			// Short read or mid-chunk failure: retry the missing remainder.
		}
		return fmt.Errorf("chunk %d: download failed after %d retries", i+1, s3PartRetries)
	})
	if err != nil {
		return err
	}
	return f.Close()
}

// readRangeInto streams one ranged-GET body to f at the given offset until
// EOF, updating progress. Returns bytes written and the terminal read error.
func (s *S3Session) readRangeInto(task *TransferTask, rc io.ReadCloser, f *os.File, offset int64) (int64, error) {
	defer rc.Close()
	buf := make([]byte, s3DownloadBufSize)
	var got int64
	for {
		select {
		case <-task.ctx.Done():
			return got, task.ctx.Err()
		default:
		}
		task.waitIfPaused()
		n, e := rc.Read(buf)
		if n > 0 {
			if _, err := f.WriteAt(buf[:n], offset+got); err != nil {
				return got, err
			}
			got += int64(n)
			task.addProgress(int64(n))
			s.emitTransferProgress(task)
		}
		if e != nil {
			if e == io.EOF {
				return got, nil
			}
			return got, e
		}
	}
}

// rangeGet issues one ranged GET by its raw S3 key. The request goes out as a
// presigned URL (simples3's FileDownload cannot set a Range header); the
// Range header itself is unsigned, which S3 and MinIO accept.
func (s *S3Session) rangeGet(key string, start, end int64) (io.ReadCloser, error) {
	urlStr := s.s3.GeneratePresignedURL(simples3.PresignedInput{
		Bucket:        s.bucket,
		ObjectKey:     key,
		Method:        http.MethodGet,
		ExpirySeconds: 3600,
	})
	if urlStr == "" {
		return nil, fmt.Errorf("failed to generate presigned URL")
	}
	req, err := http.NewRequest(http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end-1))
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusPartialContent {
		return resp.Body, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	resp.Body.Close()
	return nil, &s3RangeStatusError{code: resp.StatusCode, status: resp.Status, body: string(body)}
}

func (s *S3Session) uploadDir(localDir, remoteDir string, task *TransferTask) error {
	type ulItem struct {
		lp, rp, rel string
		size        int64
	}
	var items []ulItem
	var total int64
	err := filepath.WalkDir(localDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localDir, p)
		if err != nil {
			return err
		}
		var size int64
		if info, err := d.Info(); err == nil {
			size = info.Size()
			total += size
		}
		items = append(items, ulItem{p, path.Join(remoteDir, filepath.ToSlash(rel)), rel, size})
		return nil
	})
	if err != nil {
		return err
	}
	if task.loadTotal() <= 0 {
		task.setTotal(total)
	}
	task.setFileCount(len(items))

	return runPool(task.ctx, len(items), s3TransferConcurrency, func(i int) error {
		item := items[i]
		if task.shouldSkip(item.rel) {
			// Retry checkpoint: transferred in a previous attempt.
			task.addProgress(item.size)
			task.beginFile(item.rel)
			task.finishFile(item.rel)
			return nil
		}
		task.beginFile(item.rel)
		s.emitFileStart(task, item.rel, filepath.Base(item.lp))
		if err := s.uploadFile(task, item.lp, item.rp); err != nil {
			if task.ctx.Err() != nil {
				return err // cancelled: stop the whole transfer
			}
			task.failFile(item.rel, err)
			s.emitFileFailed(task, item.rel, err)
			return nil
		}
		task.finishFile(item.rel)
		s.emitFileDone(task, item.rel)
		return nil
	})
}

// uploadFile streams a local file to S3. Large files go through concurrent
// multipart parts read straight from disk (io.SectionReader), so memory stays
// bounded no matter the file size; small ones use a single PUT.
func (s *S3Session) uploadFile(task *TransferTask, localPath, remotePath string) error {
	fi, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()

	if task.loadTotal() <= 0 {
		task.setTotal(fi.Size())
	}
	key := s.s3Key(remotePath)
	contentType := s3ContentType(filepath.Ext(localPath))

	if fi.Size() < s3MultipartThreshold {
		data, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		_, err = s.s3.FilePut(simples3.UploadInput{
			Bucket:      s.bucket,
			ObjectKey:   key,
			ContentType: contentType,
			Body:        bytes.NewReader(data),
		})
		if err == nil {
			task.addProgress(fi.Size())
			s.emitTransferProgress(task)
		}
		return err
	}
	return s.uploadMultipart(task, key, contentType, f, fi.Size())
}

// uploadMultipart streams a large file as concurrent parts. Each worker reads
// its own s3PartSize window straight from disk, so the whole file never
// enters memory at once.
func (s *S3Session) uploadMultipart(task *TransferTask, key, contentType string, f *os.File, size int64) error {
	initOut, err := s.s3.InitiateMultipartUpload(simples3.InitiateMultipartUploadInput{
		Bucket:      s.bucket,
		ObjectKey:   key,
		ContentType: contentType,
	})
	if err != nil {
		return err
	}
	partSize := s3PartSizeFor(size)
	totalParts := int((size + partSize - 1) / partSize)
	parts := make([]simples3.CompletedPart, totalParts)

	err = runPool(task.ctx, totalParts, s3TransferConcurrency, func(i int) error {
		partNum := i + 1
		start := int64(i) * partSize
		end := start + partSize
		if end > size {
			end = size
		}
		// Stagger the first round of parts so concurrent workers don't
		// complete in lockstep; progress then advances in even intervals
		// instead of jumping a whole cluster at once.
		if partNum <= s3TransferConcurrency {
			select {
			case <-time.After(time.Duration(partNum-1) * 250 * time.Millisecond):
			case <-task.ctx.Done():
				return task.ctx.Err()
			}
		}
		out, err := s.uploadPartWithRetry(task, key, initOut.UploadID, f, start, end, partNum)
		if err != nil {
			return err
		}
		parts[i] = simples3.CompletedPart{PartNumber: out.PartNumber, ETag: out.ETag}
		task.addProgress(end - start)
		s.emitTransferProgress(task)
		return nil
	})
	if err != nil {
		// Abandon the server-side upload so orphaned parts don't linger.
		_ = s.abortMultipart(key, initOut.UploadID)
		return err
	}
	if _, err := s.s3.CompleteMultipartUpload(simples3.CompleteMultipartUploadInput{
		Bucket:    s.bucket,
		ObjectKey: key,
		UploadID:  initOut.UploadID,
		Parts:     parts,
	}); err != nil {
		_ = s.abortMultipart(key, initOut.UploadID)
		return err
	}
	return nil
}

func (s *S3Session) abortMultipart(key, uploadID string) error {
	return s.s3.AbortMultipartUpload(simples3.AbortMultipartUploadInput{
		Bucket:    s.bucket,
		ObjectKey: key,
		UploadID:  uploadID,
	})
}

// uploadPartWithRetry uploads one part with retries; each attempt re-creates
// the section reader so a consumed stream never poisons a retry.
func (s *S3Session) uploadPartWithRetry(task *TransferTask, key, uploadID string, f *os.File, start, end int64, partNum int) (simples3.UploadPartOutput, error) {
	var lastErr error
	for attempt := 0; attempt <= s3PartRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond):
			case <-task.ctx.Done():
				return simples3.UploadPartOutput{}, task.ctx.Err()
			}
		}
		task.waitIfPaused()
		out, err := s.s3.UploadPart(simples3.UploadPartInput{
			Bucket:     s.bucket,
			ObjectKey:  key,
			UploadID:   uploadID,
			PartNumber: partNum,
			Body:       io.NewSectionReader(f, start, end-start),
			Size:       end - start,
		})
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !isS3Retryable(err) {
			return simples3.UploadPartOutput{}, err
		}
	}
	return simples3.UploadPartOutput{}, lastErr
}

// isS3Retryable matches the transient faults worth retrying: 5xx responses,
// timeouts, connection resets and EOF.
func isS3Retryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "status code: 5") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "EOF")
}

