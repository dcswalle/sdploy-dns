package main

import "net"

func (s *DNSServer) getOverwrite(domain string, clientIP net.IP) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.overwrites == nil {
		return "", false
	}

	if entry, exists := s.overwrites.Exact[domain]; exists {
		if ip, ok := matchOverwriteEntry(entry, clientIP); ok {
			return ip, true
		}
	}

	for _, w := range s.overwrites.Wildcards {
		if matchOneLabelWildcard(domain, w.Suffix) {
			if ip, ok := matchOverwriteEntry(w.Entry, clientIP); ok {
				return ip, true
			}
		}
	}

	return "", false
}

func matchOverwriteEntry(entry *OverwriteEntry, clientIP net.IP) (string, bool) {
	if len(entry.Subnets) == 0 && len(entry.IPs) == 0 {
		return entry.IP, true
	}

	if clientIP == nil {
		return "", false
	}

	for _, ip := range entry.IPs {
		if ip.Equal(clientIP) {
			return entry.IP, true
		}
	}

	for _, subnet := range entry.Subnets {
		if subnet.Contains(clientIP) {
			return entry.IP, true
		}
	}

	return "", false
}

func overwriteCount(index *OverwriteIndex) int {
	if index == nil {
		return 0
	}
	return len(index.Exact) + len(index.Wildcards)
}
