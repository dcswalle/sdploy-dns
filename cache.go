package main

import (
	"fmt"
	"net"
	"time"

	"github.com/miekg/dns"
)

// getCacheKey generates a cache key from the DNS question.
func getCacheKey(r *dns.Msg) string {
	if len(r.Question) == 0 {
		return ""
	}
	q := r.Question[0]
	return fmt.Sprintf("%s:%d:%d", normalizeDomain(q.Name), q.Qtype, q.Qclass)
}

// getCachedResponse retrieves a cached DNS response if it exists and is not expired.
func (s *DNSServer) getCachedResponse(r *dns.Msg, clientIP net.IP) *dns.Msg {
	// Check if caching is enabled (either positive or negative)
	if s.config.CacheTTL <= 0 && s.config.NegativeCacheTTL <= 0 {
		return nil
	}

	key := getCacheKey(r)
	if key == "" {
		return nil
	}

	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()

	entry, exists := s.cache[key]
	if !exists {
		return nil
	}

	// Check if cache entry is expired
	if time.Now().After(entry.ExpiresAt) {
		return nil
	}

	// Create a copy of the cached message for this request
	cachedMsg := entry.Message.Copy()
	cachedMsg.Id = r.Id // Use the request ID
	cachedMsg.Question = r.Question
	cachedMsg.RecursionDesired = r.RecursionDesired
	cachedMsg.CheckingDisabled = r.CheckingDisabled

	// Log cache hit with response type
	logCacheHit(s, cachedMsg, r, clientIP)
	return cachedMsg
}

// isNegativeResponse determines if a DNS response should be cached as negative.
func isNegativeResponse(resp *dns.Msg) bool {
	if resp == nil {
		return false
	}

	// NXDOMAIN - domain does not exist
	if resp.Rcode == dns.RcodeNameError {
		return true
	}

	// SERVFAIL - server failure (delegation issues, etc.)
	if resp.Rcode == dns.RcodeServerFailure {
		return true
	}

	// REFUSED - server refuses to answer (policy rejection)
	if resp.Rcode == dns.RcodeRefused {
		return true
	}

	// NOTIMP - not implemented (rare, but cacheable)
	if resp.Rcode == dns.RcodeNotImplemented {
		return true
	}

	// NOERROR with no answers - domain exists but no records
	if resp.Rcode == dns.RcodeSuccess && len(resp.Answer) == 0 {
		return true
	}

	return false
}

// setCachedResponse stores a DNS response in the cache.
func (s *DNSServer) setCachedResponse(r *dns.Msg, resp *dns.Msg) {
	if resp == nil {
		return
	}

	key := getCacheKey(r)
	if key == "" {
		return
	}

	// Validate response matches query
	if !validateResponse(r, resp) {
		s.debugLog("Response validation failed for %s, not caching", normalizeDomain(r.Question[0].Name))
		return
	}

	// Handle all negative response types
	if isNegativeResponse(resp) {
		s.cacheNegativeResponse(r, resp, key)
		return
	}

	// Handle successful responses with answers
	s.cachePositiveResponse(r, resp, key)
}

// cacheNegativeResponse caches NXDOMAIN or NOERROR with no answers responses.
func (s *DNSServer) cacheNegativeResponse(r *dns.Msg, resp *dns.Msg, key string) {
	// Check if negative caching is enabled
	negativeTTL := s.config.NegativeCacheTTL
	if negativeTTL <= 0 {
		return // Negative caching disabled
	}

	// Try to extract TTL from SOA record's minimum TTL
	ttl := negativeTTL
	if len(resp.Ns) > 0 {
		for _, rr := range resp.Ns {
			if soa, ok := rr.(*dns.SOA); ok {
				// Use SOA's minimum TTL if available and smaller than configured TTL
				if soa.Minttl > 0 && int(soa.Minttl) < ttl {
					ttl = int(soa.Minttl)
				}
				break
			}
		}
		// If no SOA found but we have authority records, use their TTL
		if ttl == negativeTTL {
			for _, rr := range resp.Ns {
				if rr.Header().Ttl > 0 && int(rr.Header().Ttl) < ttl {
					ttl = int(rr.Header().Ttl)
				}
			}
		}
	}

	// Don't cache if TTL is too short
	if ttl < 1 {
		return
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	// Enforce cache size limit if configured
	if s.maxCacheSize > 0 && len(s.cache) >= s.maxCacheSize {
		// Remove oldest entries (simple FIFO - remove first expired, or random if none expired)
		s.evictOldestCacheEntry()
	}

	cachedMsg := resp.Copy()
	s.cache[key] = &CacheEntry{
		Message:   cachedMsg,
		ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}

	logCachedNegative(s, resp, r, ttl)
}

// cachePositiveResponse caches successful DNS responses.
func (s *DNSServer) cachePositiveResponse(r *dns.Msg, resp *dns.Msg, key string) {
	// Handle successful responses
	if s.config.CacheTTL <= 0 {
		return
	}

	// Don't cache errors (but allow NXDOMAIN above)
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) == 0 {
		return
	}

	// Determine cache TTL from response or use configured TTL
	ttl := s.config.CacheTTL
	if len(resp.Answer) > 0 {
		// Use minimum TTL from answer records
		const maxUint32 = 4294967295
		var minTTL uint32 = maxUint32
		if ttl > 0 && ttl <= maxUint32 {
			minTTL = uint32(ttl)
		}
		for _, rr := range resp.Answer {
			if rr.Header().Ttl < minTTL {
				minTTL = rr.Header().Ttl
			}
		}
		// Use the smaller of response TTL or configured TTL
		if minTTL < maxUint32 && int(minTTL) < ttl {
			ttl = int(minTTL)
		}
	}

	// Don't cache if TTL is too short
	if ttl < 1 {
		return
	}

	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	// Enforce cache size limit if configured
	if s.maxCacheSize > 0 && len(s.cache) >= s.maxCacheSize {
		// Remove oldest entries (simple FIFO - remove first expired, or random if none expired)
		s.evictOldestCacheEntry()
	}

	// Create a copy of the response for caching
	cachedMsg := resp.Copy()
	s.cache[key] = &CacheEntry{
		Message:   cachedMsg,
		ExpiresAt: time.Now().Add(time.Duration(ttl) * time.Second),
	}

	s.debugLog("Cached: %s (TTL: %ds)", normalizeDomain(r.Question[0].Name), ttl)
}

// evictOldestCacheEntry removes the oldest cache entry when cache is full.
func (s *DNSServer) evictOldestCacheEntry() {
	now := time.Now()
	var oldestKey string
	var oldestTime time.Time
	found := false

	// Find oldest entry
	for key, entry := range s.cache {
		if !found || entry.ExpiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.ExpiresAt
			found = true
		}
	}

	// If all entries are expired, prefer removing expired ones
	if found && now.After(oldestTime) {
		delete(s.cache, oldestKey)
		return
	}

	// Otherwise remove the oldest non-expired entry
	if found {
		delete(s.cache, oldestKey)
	}
}

// validateResponse checks if a DNS response matches the query.
func validateResponse(r *dns.Msg, resp *dns.Msg) bool {
	if r == nil || resp == nil {
		return false
	}

	// Check if response has questions
	if len(resp.Question) == 0 || len(r.Question) == 0 {
		return false
	}

	// Response question should match request question
	reqQ := normalizeDomain(r.Question[0].Name)
	respQ := normalizeDomain(resp.Question[0].Name)
	if reqQ != respQ {
		return false
	}

	// Response question type and class should match
	if r.Question[0].Qtype != resp.Question[0].Qtype {
		return false
	}
	if r.Question[0].Qclass != resp.Question[0].Qclass {
		return false
	}

	return true
}

// logCacheHit logs a cache hit with appropriate response type information.
func logCacheHit(s *DNSServer, cachedMsg *dns.Msg, r *dns.Msg, clientIP net.IP) {
	if len(r.Question) == 0 {
		return
	}
	domain := normalizeDomain(r.Question[0].Name)

	switch {
	case cachedMsg.Rcode == dns.RcodeNameError:
		s.debugLog("Cache hit (NXDOMAIN): %s (from %s)", domain, clientIP)
	case cachedMsg.Rcode == dns.RcodeServerFailure:
		s.debugLog("Cache hit (SERVFAIL): %s (from %s)", domain, clientIP)
	case cachedMsg.Rcode == dns.RcodeRefused:
		s.debugLog("Cache hit (REFUSED): %s (from %s)", domain, clientIP)
	case cachedMsg.Rcode == dns.RcodeNotImplemented:
		s.debugLog("Cache hit (NOTIMP): %s (from %s)", domain, clientIP)
	case cachedMsg.Rcode == dns.RcodeSuccess && len(cachedMsg.Answer) == 0:
		s.debugLog("Cache hit (NOERROR, no answers): %s (from %s)", domain, clientIP)
	default:
		s.debugLog("Cache hit: %s (from %s)", domain, clientIP)
	}
}

// logCachedNegative logs when a negative response is cached.
func logCachedNegative(s *DNSServer, resp *dns.Msg, r *dns.Msg, ttl int) {
	if len(r.Question) == 0 {
		return
	}
	domain := normalizeDomain(r.Question[0].Name)

	switch resp.Rcode {
	case dns.RcodeNameError:
		s.debugLog("Cached NXDOMAIN: %s (TTL: %ds)", domain, ttl)
	case dns.RcodeServerFailure:
		s.debugLog("Cached SERVFAIL: %s (TTL: %ds)", domain, ttl)
	case dns.RcodeRefused:
		s.debugLog("Cached REFUSED: %s (TTL: %ds)", domain, ttl)
	case dns.RcodeNotImplemented:
		s.debugLog("Cached NOTIMP: %s (TTL: %ds)", domain, ttl)
	default:
		s.debugLog("Cached NOERROR (no answers): %s (TTL: %ds)", domain, ttl)
	}
}

// cleanupExpiredCache removes expired entries from the cache.
func (s *DNSServer) cleanupExpiredCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()

	now := time.Now()
	for key, entry := range s.cache {
		if now.After(entry.ExpiresAt) {
			delete(s.cache, key)
		}
	}
}

// startCacheCleanup starts a goroutine to periodically clean up expired cache entries.
func (s *DNSServer) startCacheCleanup() {
	if s.config.CacheTTL <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			s.cleanupExpiredCache()
		}
	}()
}
