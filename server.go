package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// NewDNSServer creates a new DNS server instance.
func NewDNSServer(config *Config) (*DNSServer, error) {
	// Parse nameservers
	nameservers, err := parseNameservers(config.Nameservers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse nameservers: %w", err)
	}

	// Parse overwrites
	overwrites, err := parseOverwrites(config.Overwrites)
	if err != nil {
		return nil, fmt.Errorf("failed to parse overwrites: %w", err)
	}

	// Create server instance
	server := createDNSServerInstance(config, nameservers, overwrites)

	// Load block lists into memory (supports both file paths and conditional blocks)
	if err := server.loadBlockLists(); err != nil {
		return nil, fmt.Errorf("failed to load block lists: %w", err)
	}

	// Start background goroutines
	server.startBackgroundServices()

	return server, nil
}

// createDNSServerInstance creates and initializes a DNS server instance.
func createDNSServerInstance(config *Config, nameservers []NameserverConfig, overwrites map[string]*OverwriteEntry) *DNSServer {
	// Create HTTP client with DNS fallback support
	httpClient := createHTTPClientWithDNSFallback(config.FallbackDNS)

	return &DNSServer{
		config:          config,
		blocked:         make(map[string]*BlockEntry),
		overwrites:      overwrites,
		nameservers:     nameservers,
		cache:           make(map[string]*CacheEntry),
		maxCacheSize:    config.MaxCacheSize,
		pendingRequests: make(map[string]*PendingRequest),
		urlBlockLists:   make([]URLBlockList, 0),
		client:     &dns.Client{Timeout: 5 * time.Second},
		httpClient: httpClient,
		msgPool: &sync.Pool{
			New: func() interface{} {
				return new(dns.Msg)
			},
		},
	}
}

// startBackgroundServices starts all background goroutines for the DNS server.
func (s *DNSServer) startBackgroundServices() {
	// Start cache cleanup goroutine
	s.startCacheCleanup()

	// Start pending request cleanup goroutine
	s.startPendingRequestCleanup()

	// Start block list reloader if there are URL-based lists
	reloadInterval := s.config.ReloadInterval
	if len(s.urlBlockLists) > 0 && reloadInterval > 0 {
		s.startBlockListReloader(time.Duration(reloadInterval) * time.Minute)
		log.Printf("URL-based block list reloader started (interval: %d minutes)", reloadInterval)
	}

	log.Printf("Loaded %d blocked hosts and %d DNS overwrites", len(s.blocked), len(s.overwrites))
	log.Printf("Configured %d nameservers", len(s.nameservers))
	if s.config.CacheTTL > 0 {
		log.Printf("DNS caching enabled (TTL: %ds)", s.config.CacheTTL)
	}
}

// Start starts the DNS server.
func (s *DNSServer) Start() error {
	// Create DNS server
	dnsServer := &dns.Server{
		Addr:    s.config.ListenAddr,
		Net:     "udp",
		Handler: dns.HandlerFunc(s.handleDNSRequest),
	}

	s.debugLog("Starting DNS server on %s", s.config.ListenAddr)
	for i, ns := range s.nameservers {
		log.Printf("Nameserver %d: %s:%d (%s)", i+1, ns.Address, ns.Port, ns.Protocol)
	}
	log.Printf("Block lists: %v", s.config.BlockLists)

	// Start UDP server
	if err := dnsServer.ListenAndServe(); err != nil {
		return fmt.Errorf("failed to start DNS server: %w", err)
	}

	return nil
}

// createHTTPClientWithDNSFallback creates an HTTP client with DNS fallback support.
func createHTTPClientWithDNSFallback(fallbackDNS string) *http.Client {
	// Set default fallback DNS if not configured
	if fallbackDNS == "" {
		fallbackDNS = "8.8.8.8" // Default to Google DNS
	}

	// Check if DNS is working
	dnsWorking := checkDNSWorking()

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	// If DNS is not working, use custom dialer with fallback DNS
	if !dnsWorking {
		log.Printf("System DNS not working, using fallback DNS server: %s", fallbackDNS)
		transport.DialContext = createDialContextWithFallback(fallbackDNS)
	}

	return &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second, // Longer timeout for downloading large block lists
	}
}

// createDialContextWithFallback creates a DialContext function that uses fallback DNS.
func createDialContextWithFallback(fallbackDNS string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(_ context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		// Try to resolve using fallback DNS
		addrs, err := resolveHostWithFallback(host, fallbackDNS)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve %s: %w", host, err)
		}

		// Try each resolved address
		var lastErr error
		for _, ip := range addrs {
			conn, err := net.DialTimeout(network, net.JoinHostPort(ip, port), 10*time.Second)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}

		return nil, fmt.Errorf("failed to connect to %s: %w", addr, lastErr)
	}
}

// startPendingRequestCleanup starts a goroutine to periodically clean up stale pending requests.
func (s *DNSServer) startPendingRequestCleanup() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			s.cleanupStalePendingRequests()
		}
	}()
}

// cleanupStalePendingRequests removes stale pending requests that may have been abandoned.
func (s *DNSServer) cleanupStalePendingRequests() {
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()

	// Remove empty pending requests (no waiters)
	for key, pending := range s.pendingRequests {
		pending.mu.Lock()
		if len(pending.waiters) == 0 {
			delete(s.pendingRequests, key)
		}
		pending.mu.Unlock()
	}
}
