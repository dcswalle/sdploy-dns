package main

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// parseSubnet parses a CIDR subnet string.
func parseSubnet(subnetStr string) (*net.IPNet, error) {
	if !strings.Contains(subnetStr, "/") {
		// If no CIDR notation, treat as /32 (single IP)
		subnetStr += "/32"
	}
	_, ipNet, err := net.ParseCIDR(subnetStr)
	return ipNet, err
}

// normalizeDomain normalizes a domain name for comparison.
// Uses string interning to reduce allocations.
var domainCache sync.Map

func normalizeDomain(domain string) string {
	// Fast path: check cache first
	if cached, ok := domainCache.Load(domain); ok {
		return cached.(string)
	}

	// Normalize domain
	normalized := strings.ToLower(domain)
	normalized = strings.TrimSpace(normalized)
	// Remove trailing dot if present
	normalized = strings.TrimSuffix(normalized, ".")

	// Store in cache (only if reasonable size to avoid memory bloat)
	if len(normalized) < 256 {
		domainCache.Store(domain, normalized)
		// Also store normalized->normalized for direct lookups
		if normalized != domain {
			domainCache.Store(normalized, normalized)
		}
	}

	return normalized
}

// getClientIP extracts the client IP from the DNS request.
func getClientIP(w dns.ResponseWriter) net.IP {
	remoteAddr := w.RemoteAddr()
	if remoteAddr == nil {
		return nil
	}

	// Optimize: avoid string conversion if possible
	addrStr := remoteAddr.String()
	host, _, err := net.SplitHostPort(addrStr)
	if err != nil {
		// Try parsing as IP directly (no port)
		return net.ParseIP(addrStr)
	}

	return net.ParseIP(host)
}

// isURL checks if a string is a URL (starts with http:// or https://).
func isURL(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

// checkDNSWorking checks if DNS resolution is working by trying to resolve a well-known domain.
func checkDNSWorking() bool {
	_, err := net.LookupHost("google.com")
	return err == nil
}

// resolveHostWithFallback resolves a hostname using system DNS, or falls back to a specified DNS server.
func resolveHostWithFallback(host string, fallbackDNS string) ([]string, error) {
	// First try system DNS
	addrs, err := net.LookupHost(host)
	if err == nil {
		return addrs, nil
	}

	// If system DNS fails, use fallback DNS server
	if fallbackDNS == "" {
		return nil, err
	}

	// Use miekg/dns to query the fallback DNS server
	client := &dns.Client{Timeout: 5 * time.Second}
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(host), dns.TypeA)

	resp, _, err := client.Exchange(msg, net.JoinHostPort(fallbackDNS, "53"))
	if err != nil {
		return nil, fmt.Errorf("fallback DNS resolution failed: %w", err)
	}

	if resp.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DNS query failed with Rcode %d", resp.Rcode)
	}

	var addrsFromDNS []string
	for _, answer := range resp.Answer {
		if a, ok := answer.(*dns.A); ok {
			addrsFromDNS = append(addrsFromDNS, a.A.String())
		}
	}

	if len(addrsFromDNS) == 0 {
		return nil, fmt.Errorf("no A records found for %s", host)
	}

	return addrsFromDNS, nil
}
