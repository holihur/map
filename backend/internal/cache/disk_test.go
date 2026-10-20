package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskCache(t *testing.T) {
	dir := t.TempDir()
	c := NewDiskCache(dir, time.Minute, 0)
	if !c.Enabled() {
		t.Fatal("expected enabled")
	}
	if err := c.Set("countries/WebMercatorQuad/2/3/1.png", []byte("png")); err != nil {
		t.Fatal(err)
	}
	if v, ok := c.Get("countries/WebMercatorQuad/2/3/1.png"); !ok || string(v) != "png" {
		t.Fatalf("get = %q %v", v, ok)
	}
	// 目录穿越应被拒绝
	if _, ok := c.Get("../../../etc/passwd"); ok {
		t.Fatal("path traversal should be rejected")
	}
	if err := c.Set("../evil", []byte("x")); err == nil {
		t.Fatal("expected invalid path error")
	}
	if err := c.RemovePrefix("countries/"); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("countries/WebMercatorQuad/2/3/1.png"); ok {
		t.Fatal("expected removed")
	}
}

func TestDiskCacheExpiry(t *testing.T) {
	dir := t.TempDir()
	c := NewDiskCache(dir, 5*time.Millisecond, 0)
	_ = c.Set("a/b.png", []byte("1"))
	time.Sleep(15 * time.Millisecond)
	if _, ok := c.Get("a/b.png"); ok {
		t.Fatal("expected expired")
	}
}

func TestDiskCacheStats(t *testing.T) {
	dir := t.TempDir()
	c := NewDiskCache(dir, time.Minute, 0)
	_ = c.Set("a/1.png", []byte("1234"))
	_ = c.Set("a/2.png", []byte("12"))
	files, bytes := c.Stats()
	if files != 2 || bytes != 6 {
		t.Fatalf("stats = %d files %d bytes", files, bytes)
	}
	if err := c.Purge(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Fatal("expected purged")
	}
}

func TestDiskCacheLRUEviction(t *testing.T) {
	dir := t.TempDir()
	// 上限 20 字节；写入 3 个 10 字节文件应触发淘汰
	c := NewDiskCache(dir, time.Minute, 20)
	_ = c.Set("a/1.png", make([]byte, 10))
	time.Sleep(5 * time.Millisecond)
	_ = c.Set("a/2.png", make([]byte, 10))
	time.Sleep(5 * time.Millisecond)
	_ = c.Set("a/3.png", make([]byte, 10))

	_, bytes := c.Stats()
	if bytes > 20 {
		t.Fatalf("cache exceeded limit: %d bytes", bytes)
	}
	if _, ok := c.Get("a/1.png"); ok {
		t.Fatal("expected oldest entry to be evicted")
	}
}

func TestDiskCacheCleanupExpired(t *testing.T) {
	dir := t.TempDir()
	c := NewDiskCache(dir, 5*time.Millisecond, 0)
	_ = c.Set("a/1.png", []byte("x"))
	time.Sleep(15 * time.Millisecond)
	if n := c.CleanupExpired(); n != 1 {
		t.Fatalf("cleanup removed %d, want 1", n)
	}
}
