package cache

import (
	"testing"
	"time"
)

func TestSetGet(t *testing.T) {
	c := NewByteCache(2, time.Minute)
	c.Set("a", []byte("1"))
	if v, ok := c.Get("a"); !ok || string(v) != "1" {
		t.Fatalf("expected hit, got %q %v", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss")
	}
}

func TestLRUEviction(t *testing.T) {
	c := NewByteCache(2, time.Minute)
	c.Set("a", []byte("1"))
	c.Set("b", []byte("2"))
	// 访问 a，使 b 成为最久未使用
	_, _ = c.Get("a")
	c.Set("c", []byte("3"))
	if _, ok := c.Get("b"); ok {
		t.Fatal("expected b to be evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected a to remain")
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("expected c to remain")
	}
}

func TestExpiry(t *testing.T) {
	c := NewByteCache(4, 10*time.Millisecond)
	c.Set("k", []byte("v"))
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("expected expired entry to miss")
	}
}

func TestInvalidatePrefix(t *testing.T) {
	c := NewByteCache(0, time.Minute)
	c.Set("cities/1/1/1", []byte("a"))
	c.Set("cities/2/2/2", []byte("b"))
	c.Set("roads/1/1/1", []byte("c"))
	c.InvalidatePrefix("cities/")
	if _, ok := c.Get("cities/1/1/1"); ok {
		t.Fatal("expected cities cache invalidated")
	}
	if _, ok := c.Get("roads/1/1/1"); !ok {
		t.Fatal("expected roads cache intact")
	}
}

func TestStats(t *testing.T) {
	c := NewByteCache(4, time.Minute)
	c.Set("x", []byte("1"))
	_, _ = c.Get("x")
	_, _ = c.Get("y")
	hits, misses, size, _ := c.Stats()
	if hits != 1 || misses != 1 || size != 1 {
		t.Fatalf("stats = hits:%d misses:%d size:%d", hits, misses, size)
	}
}
