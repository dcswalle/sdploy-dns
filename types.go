package main

import (
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// NameserverConfig represents a nameserver with protocol.
type NameserverConfig struct {
	Address  string `yaml:"address"`
	Protocol string `yaml:"protocol"` // udp, tcp, dot, doh
	Port     int    `yaml:"port"`     // Optional, defaults based on protocol
}

// OverwriteConfig represents a DNS overwrite with optional IP/subnet conditions.
type OverwriteConfig struct {
	IP      string   `yaml:"ip"`      // IP address to return
	Subnets []string `yaml:"subnets"` // Optional: only apply to these subnets
	IPs     []string `yaml:"ips"`     // Optional: only apply to these specific IPs
}

// Config represents the DNS server configuration.
type Config struct {
	ListenAddr        string                 `yaml:"listen_addr"`
	Nameservers       interface{}            `yaml:"nameservers"`        // Can be []string or []NameserverConfig
	Overwrites        map[string]interface{} `yaml:"overwrites"`        // Can be string or OverwriteConfig
	BlockLists        interface{}            `yaml:"block_lists"`        // Can be []string or []interface{} with conditional blocks
	CacheTTL          int                    `yaml:"cache_ttl"`         // Cache TTL in seconds (default: 60)
	NegativeCacheTTL  int                    `yaml:"negative_cache_ttl"` // Negative cache TTL for NXDOMAIN in seconds (default: 300, set to 0 to disable)
	MaxCacheSize      int                    `yaml:"max_cache_size"`    // Maximum cache entries (default: 0 = unlimited)
	ReloadInterval    int                    `yaml:"reload_interval"`   // Reload interval for URL-based block lists in minutes (default: 60)
	FallbackDNS       string                 `yaml:"fallback_dns"`      // Fallback DNS server for downloading block lists (default: "8.8.8.8")
	GOGC              int                    `yaml:"gogc"`             // GOGC value for GC tuning (default: 100, set to 0 to use Go default)
	Debug             bool                   `yaml:"debug"`             // Enable debug logging (default: false)
	LogBlocks         bool                   `yaml:"log_blocks"`        // Log blocked requests (default: false)
	LogOverwrites     bool                   `yaml:"log_overwrites"`    // Log overwritten requests (default: false)
}

// OverwriteEntry represents a parsed overwrite entry.
type OverwriteEntry struct {
	IP      string     // IP address to return (from first element of ips if conditional)
	Subnets []*net.IPNet
	IPs     []net.IP   // Client IPs to match (first IP is also used as return IP if no simple IP set)
}

// BlockEntry represents a parsed block entry with optional IP/subnet restrictions.
type BlockEntry struct {
	Subnets []*net.IPNet // Optional: only block for these subnets
	IPs     []net.IP     // Optional: only block for these specific IPs
}

// URLBlockList represents a URL-based block list with its restrictions.
type URLBlockList struct {
	URL          string
	Restrictions *BlockEntry
}

// CacheEntry represents a cached DNS response.
type CacheEntry struct {
	Message   *dns.Msg
	ExpiresAt time.Time
}

// PendingRequest represents a pending DNS request waiting for a response.
type PendingRequest struct {
	waiters []chan *dns.Msg
	mu      sync.Mutex
}

// DNSServer represents the DNS server instance.
//
// Lock ordering: To prevent deadlock, locks must be acquired in this order:
//  1. pendingMu (always released before acquiring cacheMu)
//  2. cacheMu (never held while acquiring pendingMu)
// The locks are never held simultaneously.
type DNSServer struct {
	config        *Config
	blocked       map[string]*BlockEntry // Changed to support conditional blocking
	overwrites    map[string]*OverwriteEntry
	nameservers   []NameserverConfig
	cache         map[string]*CacheEntry // DNS response cache
	cacheMu       sync.RWMutex           // Cache mutex - see lock ordering above
	maxCacheSize  int                    // Maximum cache entries (0 = unlimited)
	mu            sync.RWMutex
	pendingRequests map[string]*PendingRequest // Track pending requests for coalescing
	pendingMu     sync.Mutex                   // Pending requests mutex - see lock ordering above
	urlBlockLists []URLBlockList // Track URL-based block lists for reloading
	client        *dns.Client
	httpClient    *http.Client
	msgPool       *sync.Pool // Pool for dns.Msg objects
	nameserverIdx uint64      // Atomic counter for round-robin nameserver selection
}
