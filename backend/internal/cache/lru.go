// Package cache 提供一个带 TTL 的线程安全 LRU 字节缓存。
package cache

import (
	"container/list"
	"strings"
	"sync"
	"time"
)

type entry struct {
	key     string
	value   []byte
	expires time.Time
}

// ByteCache 是一个带过期时间与容量上限的 LRU 缓存。
type ByteCache struct {
	mu      sync.Mutex
	max     int
	ttl     time.Duration
	ll      *list.List
	items   map[string]*list.Element
	hits    int64
	misses  int64
	evicted int64
}

// NewByteCache 创建缓存；max<=0 表示不限制容量。
func NewByteCache(max int, ttl time.Duration) *ByteCache {
	return &ByteCache{
		max:   max,
		ttl:   ttl,
		ll:    list.New(),
		items: make(map[string]*list.Element),
	}
}

// Get 读取缓存，命中且未过期时返回副本。
func (c *ByteCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		c.misses++
		return nil, false
	}
	e := el.Value.(*entry)
	if c.ttl > 0 && time.Now().After(e.expires) {
		c.ll.Remove(el)
		delete(c.items, key)
		c.misses++
		return nil, false
	}
	c.ll.MoveToFront(el)
	c.hits++
	out := make([]byte, len(e.value))
	copy(out, e.value)
	return out, true
}

// Set 写入缓存。
func (c *ByteCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		e := el.Value.(*entry)
		e.value = append(e.value[:0], value...)
		if c.ttl > 0 {
			e.expires = time.Now().Add(c.ttl)
		}
		c.ll.MoveToFront(el)
		return
	}
	e := &entry{key: key, value: append([]byte(nil), value...)}
	if c.ttl > 0 {
		e.expires = time.Now().Add(c.ttl)
	}
	c.items[key] = c.ll.PushFront(e)
	for c.max > 0 && c.ll.Len() > c.max {
		c.removeOldest()
	}
}

// InvalidatePrefix 删除所有以 prefix 开头的键。
func (c *ByteCache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key, el := range c.items {
		if strings.HasPrefix(key, prefix) {
			c.ll.Remove(el)
			delete(c.items, key)
		}
	}
}

// Purge 清空缓存。
func (c *ByteCache) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element)
}

// Stats 返回命中、未命中、当前条目数与淘汰数。
func (c *ByteCache) Stats() (hits, misses, size, evicted int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses, int64(c.ll.Len()), c.evicted
}

func (c *ByteCache) removeOldest() {
	el := c.ll.Back()
	if el == nil {
		return
	}
	c.ll.Remove(el)
	delete(c.items, el.Value.(*entry).key)
	c.evicted++
}
