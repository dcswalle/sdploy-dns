package main

import (
	"fmt"

	"github.com/miekg/dns"
)

// handleDNSRequest handles incoming DNS requests.
func (s *DNSServer) handleDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	// Get client IP early for cache logging
	clientIP := getClientIP(w)

	// Check cache first - fastest path for cached responses
	if cachedResp := s.getCachedResponse(r, clientIP); cachedResp != nil {
		if err := w.WriteMsg(cachedResp); err != nil {
			errorLog("Error writing cached response: %v", err)
		}
		return
	}

	// Normalize domain once
	domain := normalizeDomain(r.Question[0].Name)

	// Check if domain is blocked (with IP/subnet matching)
	if s.isBlocked(domain, clientIP) {
		s.logBlock("Blocked: %s (from %s)", domain, clientIP)
		// Return NXDOMAIN for blocked domains
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Authoritative = true
		msg.SetRcode(r, dns.RcodeNameError)
		if err := w.WriteMsg(msg); err != nil {
			errorLog("Error writing response: %v", err)
		}
		return
	}

	// Check for DNS overwrite (with IP/subnet matching)
	if ip, exists := s.getOverwrite(domain, clientIP); exists {
		s.logOverwrite("Overwrite: %s -> %s (for client %s)", domain, ip, clientIP)
		// Create A record response
		msg := new(dns.Msg)
		msg.SetReply(r)
		msg.Authoritative = true
		rr, err := dns.NewRR(fmt.Sprintf("%s 300 IN A %s", r.Question[0].Name, ip))
		if err == nil {
			msg.Answer = append(msg.Answer, rr)
			if err := w.WriteMsg(msg); err != nil {
				errorLog("Error writing response: %v", err)
			}
			return
		}
	}

	// Forward to upstream nameservers
	s.forwardRequest(w, r, domain, clientIP)
}
