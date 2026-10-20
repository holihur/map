package cache

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DiskCache 是基于文件系统的瓦片缓存（TTL + 容量上限 + LRU 淘汰）。
type DiskCache struct {
	root     string
	ttl      time.Duration
	maxBytes int64
	mu       sync.Mutex
	curBytes int64
}

// NewDiskCache 创建磁盘缓存；root 为空表示禁用，maxBytes<=0 表示不限制容量。
func NewDiskCache(root string, ttl time.Duration, maxBytes int64) *DiskCache {
	d := &DiskCache{root: root, ttl: ttl, maxBytes: maxBytes}
	if d.Enabled() {
		d.curBytes = d.walkSize()
	}
	return d
}

// Enabled 表示是否启用磁盘缓存。
func (d *DiskCache) Enabled() bool { return d != nil && d.root != "" }

// Root 返回缓存根目录。
func (d *DiskCache) Root() string {
	if d == nil {
		return ""
	}
	return d.root
}

func (d *DiskCache) path(key string) (string, bool) {
	clean := filepath.Clean(filepath.FromSlash(key))
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", false
	}
	return filepath.Join(d.root, clean), true
}

// Get 读取缓存文件，过期或不存在时返回 false。
func (d *DiskCache) Get(key string) ([]byte, bool) {
	if !d.Enabled() {
		return nil, false
	}
	p, ok := d.path(key)
	if !ok {
		return nil, false
	}
	info, err := os.Stat(p)
	if err != nil || info.IsDir() {
		return nil, false
	}
	if d.ttl > 0 && time.Since(info.ModTime()) > d.ttl {
		d.removeFile(p, info.Size())
		return nil, false
	}
	// 触碰 mtime 以支持 LRU
	now := time.Now()
	_ = os.Chtimes(p, now, now)

	data, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	return data, true
}

// Set 原子写入缓存文件，并在超过容量上限时按 LRU 淘汰。
func (d *DiskCache) Set(key string, data []byte) error {
	if !d.Enabled() {
		return nil
	}
	p, ok := d.path(key)
	if !ok {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		return err
	}
	d.mu.Lock()
	d.curBytes += int64(len(data))
	over := d.maxBytes > 0 && d.curBytes > d.maxBytes
	d.mu.Unlock()

	if over {
		d.evictLRU()
	}
	return nil
}

// evictLRU 删除最久未使用的文件，直到低于容量上限的 90%。
func (d *DiskCache) evictLRU() {
	type fileInfo struct {
		path string
		size int64
		mod  time.Time
	}
	var files []fileInfo
	_ = filepath.WalkDir(d.root, func(p string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			files = append(files, fileInfo{path: p, size: info.Size(), mod: info.ModTime()})
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })

	target := d.maxBytes * 9 / 10
	d.mu.Lock()
	cur := d.curBytes
	d.mu.Unlock()
	for _, f := range files {
		if cur <= target {
			break
		}
		if err := os.Remove(f.path); err == nil {
			cur -= f.size
		}
	}
	d.mu.Lock()
	d.curBytes = cur
	d.mu.Unlock()
}

// CleanupExpired 删除所有已过期的缓存文件，返回删除数量。
func (d *DiskCache) CleanupExpired() int {
	if !d.Enabled() || d.ttl <= 0 {
		return 0
	}
	removed := 0
	_ = filepath.WalkDir(d.root, func(p string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if time.Since(info.ModTime()) > d.ttl {
			d.removeFile(p, info.Size())
			removed++
		}
		return nil
	})
	return removed
}

func (d *DiskCache) removeFile(path string, size int64) {
	if err := os.Remove(path); err == nil {
		d.mu.Lock()
		d.curBytes -= size
		if d.curBytes < 0 {
			d.curBytes = 0
		}
		d.mu.Unlock()
	}
}

// RemovePrefix 删除某前缀（如 "layer/"）下的所有缓存。
func (d *DiskCache) RemovePrefix(prefix string) error {
	if !d.Enabled() {
		return nil
	}
	dir := filepath.Join(d.root, filepath.Clean(filepath.FromSlash(prefix)))
	root := filepath.Clean(d.root)
	if !strings.HasPrefix(dir, root) || dir == root {
		return os.ErrInvalid
	}
	err := os.RemoveAll(dir)
	d.Recount()
	return err
}

// Purge 清空整个缓存目录。
func (d *DiskCache) Purge() error {
	if !d.Enabled() {
		return nil
	}
	entries, err := os.ReadDir(d.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(d.root, e.Name()))
	}
	d.mu.Lock()
	d.curBytes = 0
	d.mu.Unlock()
	return nil
}

// Stats 返回缓存文件数与总字节数。
func (d *DiskCache) Stats() (files, bytes int64) {
	if !d.Enabled() {
		return 0, 0
	}
	_ = filepath.WalkDir(d.root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			files++
			bytes += info.Size()
		}
		return nil
	})
	return files, bytes
}

// Recount 重新统计缓存占用字节数。
func (d *DiskCache) Recount() {
	if !d.Enabled() {
		return
	}
	d.mu.Lock()
	d.curBytes = d.walkSize()
	d.mu.Unlock()
}

func (d *DiskCache) walkSize() int64 {
	var total int64
	_ = filepath.WalkDir(d.root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}
