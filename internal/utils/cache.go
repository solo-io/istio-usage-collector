package utils

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// we create a global cache for the matched selector info,
// because this program is only ran once, we don't need to worry about the cache
var cache = NewMatchedSelectorInfoCache()

// MatchedSelectorInfo is your existing result struct
type MatchedSelectorInfo struct {
	Matched bool
	Label   string
}

// matchedCacheKey combines selector identity + a fingerprint of the labels map
type matchedCacheKey struct {
	sel       *metav1.LabelSelector
	labelsKey string
}

// MatchedSelectorInfoCache is safe for concurrent use
type MatchedSelectorInfoCache struct {
	mu   sync.RWMutex
	data map[matchedCacheKey]*MatchedSelectorInfo
}

func NewMatchedSelectorInfoCache() *MatchedSelectorInfoCache {
	fmt.Println("NewMatchedSelectorInfoCache")
	return &MatchedSelectorInfoCache{
		data: make(map[matchedCacheKey]*MatchedSelectorInfo, 256),
	}
}

func (c *MatchedSelectorInfoCache) get(sel *metav1.LabelSelector, objLabels map[string]string) (*MatchedSelectorInfo, bool) {
	fmt.Println("get")
	key := matchedCacheKey{sel: sel, labelsKey: fingerprint(objLabels)}
	c.mu.RLock()
	info, ok := c.data[key]
	c.mu.RUnlock()
	fmt.Println("get", info, ok)
	return info, ok
}

func (c *MatchedSelectorInfoCache) set(sel *metav1.LabelSelector, objLabels map[string]string, info *MatchedSelectorInfo) {
	fmt.Println("set")
	key := matchedCacheKey{sel: sel, labelsKey: fingerprint(objLabels)}
	c.mu.Lock()
	c.data[key] = info
	c.mu.Unlock()
	fmt.Println("set", key, info)
}

// fingerprint turns a map into a stable string so it can be used as a key
func fingerprint(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
		b.WriteByte(';')
	}
	return b.String()
}
