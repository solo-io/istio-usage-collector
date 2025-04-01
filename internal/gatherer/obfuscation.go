package gatherer

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"sync"
)

// ObfuscationCache is a cache of obfuscated names to avoid recalculating
var ObfuscationCache sync.Map

// ObfuscateName obfuscates a name using SHA-256
func ObfuscateName(name string) string {
	if name == "" {
		return ""
	}

	// Check cache first
	if cachedName, ok := ObfuscationCache.Load(name); ok {
		return cachedName.(string)
	}

	// Generate hash for uncached names
	h := sha256.New()
	io.WriteString(h, name)

	// Use only the first 16 bytes for shorter names
	// (still provides sufficient uniqueness while improving readability)
	result := strings.ToLower(hex.EncodeToString(h.Sum(nil)[:16]))

	// Store in cache
	ObfuscationCache.Store(name, result)

	return result
}
