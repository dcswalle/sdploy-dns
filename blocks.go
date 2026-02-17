package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// loadBlockLists loads adblock-style host files with per-file IP/subnet restrictions.
func (s *DNSServer) loadBlockLists() error {
	if s.config.BlockLists == nil {
		return nil
	}

	switch blockLists := s.config.BlockLists.(type) {
	case []interface{}:
		// New format: can contain strings (file paths) or maps (file with restrictions)
		for _, item := range blockLists {
			switch v := item.(type) {
			case string:
				// Simple file path - load from file with no restrictions
				if err := s.loadBlockListFile(v, nil); err != nil {
					log.Printf("Warning: failed to load block list %s: %v", v, err)
					// Continue loading other files even if one fails
				}
			case map[string]interface{}:
				// File entry with restrictions
				if err := s.loadBlockListFileWithRestrictions(v); err != nil {
					log.Printf("Warning: failed to load block list entry: %v", err)
				}
			case map[interface{}]interface{}:
				// File entry with restrictions (fallback)
				if err := s.loadBlockListFileWithRestrictionsMap(v); err != nil {
					log.Printf("Warning: failed to load block list entry: %v", err)
				}
			}
		}
	case []string:
		// Old format: array of file paths (no restrictions)
		for _, filePath := range blockLists {
			if err := s.loadBlockListFile(filePath, nil); err != nil {
				log.Printf("Warning: failed to load block list %s: %v", filePath, err)
				// Continue loading other files even if one fails
			}
		}
	default:
		return fmt.Errorf("invalid block_lists format")
	}

	return nil
}

// loadBlockListFileWithRestrictions loads a file with IP/subnet restrictions.
func (s *DNSServer) loadBlockListFileWithRestrictions(entry map[string]interface{}) error {
	filePath, ok := entry["file"].(string)
	if !ok {
		return fmt.Errorf("missing 'file' field in block list entry")
	}

	// Parse restrictions
	restrictions := &BlockEntry{}
	if subnets, ok := entry["subnets"].([]interface{}); ok {
		for _, subnetStr := range subnets {
			if subnet, ok := subnetStr.(string); ok {
				ipNet, err := parseSubnet(subnet)
				if err != nil {
					return fmt.Errorf("invalid subnet %s: %w", subnet, err)
				}
				restrictions.Subnets = append(restrictions.Subnets, ipNet)
			}
		}
	}

	if ips, ok := entry["ips"].([]interface{}); ok {
		for _, ipStr := range ips {
			if ipStr, ok := ipStr.(string); ok {
				ip := net.ParseIP(ipStr)
				if ip != nil {
					restrictions.IPs = append(restrictions.IPs, ip)
				}
			}
		}
	}

	// Load file with restrictions
	return s.loadBlockListFile(filePath, restrictions)
}

// loadBlockListFileWithRestrictionsMap loads a file with IP/subnet restrictions (fallback).
func (s *DNSServer) loadBlockListFileWithRestrictionsMap(entry map[interface{}]interface{}) error {
	filePath, ok := entry["file"].(string)
	if !ok {
		return fmt.Errorf("missing 'file' field in block list entry")
	}

	// Parse restrictions
	restrictions := &BlockEntry{}
	if subnets, ok := entry["subnets"].([]interface{}); ok {
		for _, subnetStr := range subnets {
			if subnet, ok := subnetStr.(string); ok {
				ipNet, err := parseSubnet(subnet)
				if err != nil {
					return fmt.Errorf("invalid subnet %s: %w", subnet, err)
				}
				restrictions.Subnets = append(restrictions.Subnets, ipNet)
			}
		}
	}

	if ips, ok := entry["ips"].([]interface{}); ok {
		for _, ipStr := range ips {
			if ipStr, ok := ipStr.(string); ok {
				ip := net.ParseIP(ipStr)
				if ip != nil {
					restrictions.IPs = append(restrictions.IPs, ip)
				}
			}
		}
	}

	// Load file with restrictions
	return s.loadBlockListFile(filePath, restrictions)
}

// loadBlockListFile loads a single adblock-style host file or URL with optional restrictions.
// The function ensures proper resource cleanup via defer, which executes on both success
// and error paths, including any errors returned by processBlockListReader.
func (s *DNSServer) loadBlockListFile(filePath string, restrictions *BlockEntry) error {
	reader, sourceName, closer, err := s.getBlockListReader(filePath, restrictions)
	if err != nil {
		return err
	}
	// Defer ensures the reader/connection is closed on all return paths (success or error).
	defer func() {
		if closer != nil {
			if closeErr := closer.Close(); closeErr != nil {
				s.debugLog("Warning: failed to close %s: %v", sourceName, closeErr)
			}
		}
	}()

	return s.processBlockListReader(reader, sourceName, restrictions)
}

// getBlockListReader returns a reader for a block list file or URL.
func (s *DNSServer) getBlockListReader(filePath string, restrictions *BlockEntry) (io.Reader, string, io.Closer, error) {
	if isURL(filePath) {
		return s.getURLReader(filePath, restrictions)
	}
	return s.getFileReader(filePath)
}

// getURLReader downloads a block list from a URL and returns a reader.
func (s *DNSServer) getURLReader(filePath string, restrictions *BlockEntry) (io.Reader, string, io.Closer, error) {
	resp, err := s.httpClient.Get(filePath)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to download %s: %w", filePath, err)
	}

	if resp.StatusCode != http.StatusOK {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.debugLog("Warning: failed to close response body for %s: %v", filePath, closeErr)
		}
		return nil, "", nil, fmt.Errorf("failed to download %s: HTTP %d", filePath, resp.StatusCode)
	}

	// Track URL-based block lists for periodic reloading (only if not already tracked)
	s.trackURLBlockList(filePath, restrictions)

	return resp.Body, filePath, resp.Body, nil
}

// trackURLBlockList adds a URL to the tracking list if it's not already there.
func (s *DNSServer) trackURLBlockList(filePath string, restrictions *BlockEntry) {
	// Check if URL is already tracked
	for _, existing := range s.urlBlockLists {
		if existing.URL == filePath {
			// URL already tracked, skip
			return
		}
	}

	// Add new URL to tracking list
	if restrictions != nil {
		restrictionsCopy := &BlockEntry{
			Subnets: make([]*net.IPNet, len(restrictions.Subnets)),
			IPs:     make([]net.IP, len(restrictions.IPs)),
		}
		copy(restrictionsCopy.Subnets, restrictions.Subnets)
		copy(restrictionsCopy.IPs, restrictions.IPs)
		s.urlBlockLists = append(s.urlBlockLists, URLBlockList{
			URL:          filePath,
			Restrictions: restrictionsCopy,
		})
	} else {
		s.urlBlockLists = append(s.urlBlockLists, URLBlockList{
			URL:          filePath,
			Restrictions: nil,
		})
	}
}

// getFileReader opens a local file and returns a reader.
func (s *DNSServer) getFileReader(filePath string) (io.Reader, string, io.Closer, error) {
	cleanPath := filepath.Clean(filePath)
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, "", nil, err
	}
	return file, cleanPath, file, nil
}

// processBlockListReader processes a block list from a reader.
// Note: The caller is responsible for closing the reader. This function does not close it.
func (s *DNSServer) processBlockListReader(reader io.Reader, sourceName string, restrictions *BlockEntry) error {
	scanner := bufio.NewScanner(reader)
	lineNum := 0
	loadedCount := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		domain := s.parseHostLine(line)
		if domain != "" {
			s.addBlockedDomain(domain, restrictions)
			loadedCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading %s at line %d: %w", sourceName, lineNum, err)
	}

	s.logBlockListLoaded(sourceName, loadedCount, restrictions)
	return nil
}

// addBlockedDomain adds a domain to the blocked list with optional restrictions.
func (s *DNSServer) addBlockedDomain(domain string, restrictions *BlockEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	domain = normalizeDomain(domain)
	if restrictions != nil {
		entry := &BlockEntry{
			Subnets: make([]*net.IPNet, len(restrictions.Subnets)),
			IPs:     make([]net.IP, len(restrictions.IPs)),
		}
		copy(entry.Subnets, restrictions.Subnets)
		copy(entry.IPs, restrictions.IPs)
		s.blocked[domain] = entry
	} else {
		s.blocked[domain] = &BlockEntry{}
	}
}

// logBlockListLoaded logs the loading of a block list file with optional restrictions.
func (s *DNSServer) logBlockListLoaded(filePath string, count int, restrictions *BlockEntry) {
	if restrictions != nil {
		restrictionStr := ""
		if len(restrictions.IPs) > 0 {
			ips := make([]string, len(restrictions.IPs))
			for i, ip := range restrictions.IPs {
				ips[i] = ip.String()
			}
			restrictionStr += fmt.Sprintf(" (IPs: %v)", ips)
		}
		if len(restrictions.Subnets) > 0 {
			subnets := make([]string, len(restrictions.Subnets))
			for i, subnet := range restrictions.Subnets {
				subnets[i] = subnet.String()
			}
			restrictionStr += fmt.Sprintf(" (subnets: %v)", subnets)
		}
		log.Printf("Loaded %d domains from %s%s", count, filePath, restrictionStr)
	} else {
		log.Printf("Loaded %d domains from %s", count, filePath)
	}
}

// parseHostLine parses a line from a host file and extracts the domain.
func (s *DNSServer) parseHostLine(line string) string {
	// Remove adblock-style prefixes
	line = strings.TrimPrefix(line, "||")
	line = strings.TrimSuffix(line, "^")
	line = strings.TrimSuffix(line, "$")

	// Split by whitespace
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return ""
	}

	// If first part is an IP address, get the domain from the second part
	if len(parts) > 1 {
		firstPart := parts[0]
		if net.ParseIP(firstPart) != nil {
			// First part is an IP, domain is in the second part
			return parts[1]
		}
	}

	// Otherwise, the first part is the domain
	domain := parts[0]

	// Remove any remaining adblock-style characters
	domain = strings.TrimPrefix(domain, "||")
	domain = strings.TrimSuffix(domain, "^")
	domain = strings.TrimSuffix(domain, "$")

	return domain
}

// isBlocked checks if a domain is blocked for the given client IP.
func (s *DNSServer) isBlocked(domain string, clientIP net.IP) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check exact match first (most common case)
	if entry, exists := s.blocked[domain]; exists {
		if s.matchesBlockEntry(entry, clientIP) {
			return true
		}
	}

	// Check subdomain matches (e.g., if ads.example.com is blocked, check example.com)
	// Optimized: use string slicing instead of Split/Join to reduce allocations
	for i := 0; i < len(domain); i++ {
		if domain[i] == '.' && i+1 < len(domain) {
			parentDomain := domain[i+1:]
			if entry, exists := s.blocked[parentDomain]; exists {
				if s.matchesBlockEntry(entry, clientIP) {
					return true
				}
			}
		}
	}

	return false
}

// matchesBlockEntry checks if a block entry applies to the given client IP.
func (s *DNSServer) matchesBlockEntry(entry *BlockEntry, clientIP net.IP) bool {
	// If no restrictions, block for all clients
	if len(entry.Subnets) == 0 && len(entry.IPs) == 0 {
		return true
	}

	// If no client IP provided, don't block (can't match restrictions)
	if clientIP == nil {
		return false
	}

	// Check if client IP matches any specific IP
	for _, ip := range entry.IPs {
		if ip.Equal(clientIP) {
			return true
		}
	}

	// Check if client IP matches any subnet
	for _, subnet := range entry.Subnets {
		if subnet.Contains(clientIP) {
			return true
		}
	}

	// Client IP doesn't match restrictions
	return false
}

// reloadURLBlockList reloads a single URL-based block list.
func (s *DNSServer) reloadURLBlockList(urlBlockList URLBlockList) error {
	// Download directly without tracking (already tracked)
	resp, err := s.httpClient.Get(urlBlockList.URL)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", urlBlockList.URL, err)
	}

	if resp.StatusCode != http.StatusOK {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.debugLog("Warning: failed to close response body for %s: %v", urlBlockList.URL, closeErr)
		}
		return fmt.Errorf("failed to download %s: HTTP %d", urlBlockList.URL, resp.StatusCode)
	}

	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.debugLog("Warning: failed to close response body for %s: %v", urlBlockList.URL, closeErr)
		}
	}()

	reader := resp.Body

	scanner := bufio.NewScanner(reader)
	lineNum := 0
	loadedCount := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		domain := s.parseHostLine(line)
		if domain != "" {
			s.addBlockedDomain(domain, urlBlockList.Restrictions)
			loadedCount++
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading %s at line %d: %w", urlBlockList.URL, lineNum, err)
	}

	log.Printf("Reloaded %d domains from %s", loadedCount, urlBlockList.URL)
	return nil
}

// startBlockListReloader starts a goroutine that periodically reloads URL-based block lists.
func (s *DNSServer) startBlockListReloader(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			log.Printf("Reloading URL-based block lists...")
			for _, urlBlockList := range s.urlBlockLists {
				if err := s.reloadURLBlockList(urlBlockList); err != nil {
					log.Printf("Warning: failed to reload block list %s: %v", urlBlockList.URL, err)
					// Continue reloading other lists even if one fails
				}
			}
			log.Printf("Finished reloading URL-based block lists")
		}
	}()
}
