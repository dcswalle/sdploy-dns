package main

import "net"

// getOverwrite returns the overwritten IP for a domain if it exists and matches client IP.
func (s *DNSServer) getOverwrite(domain string, clientIP net.IP) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Domain is already normalized in handler
	entry, exists := s.overwrites[domain]
	if !exists {
		return "", false
	}

	// If no IP/subnet restrictions, apply to all clients
	if len(entry.Subnets) == 0 && len(entry.IPs) == 0 {
		return entry.IP, true
	}

	// Check if client IP matches any specific IP
	if clientIP != nil {
		for _, ip := range entry.IPs {
			if ip.Equal(clientIP) {
				return entry.IP, true
			}
		}

		// Check if client IP matches any subnet
		for _, subnet := range entry.Subnets {
			if subnet.Contains(clientIP) {
				return entry.IP, true
			}
		}
	}

	// Client IP doesn't match restrictions
	return "", false
}
