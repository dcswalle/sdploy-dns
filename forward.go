package main

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// forwardDOH forwards a DNS request using DNS-over-HTTPS.
func (s *DNSServer) forwardDOH(r *dns.Msg, nameserver NameserverConfig) (*dns.Msg, error) {
	// Encode DNS message
	buf, err := r.Pack()
	if err != nil {
		return nil, fmt.Errorf("failed to pack DNS message: %w", err)
	}

	// Build DOH URL
	var url string
	if strings.HasPrefix(nameserver.Address, "http://") || strings.HasPrefix(nameserver.Address, "https://") {
		url = nameserver.Address
	} else {
		// Try common DOH endpoints
		switch nameserver.Address {
		case "1.1.1.1", "1.0.0.1":
			url = "https://cloudflare-dns.com/dns-query"
		case "8.8.8.8", "8.8.4.4":
			url = "https://dns.google/dns-query"
		default:
			// Default DOH endpoint format
			url = fmt.Sprintf("https://%s/dns-query", nameserver.Address)
		}
	}

	return buildDOHRequest(s, url, buf)
}

// buildDOHRequest builds and executes a DNS-over-HTTPS request.
func buildDOHRequest(s *DNSServer, url string, buf []byte) (*dns.Msg, error) {
	// Try POST first (more reliable), fallback to GET
	req, err := http.NewRequest("POST", url, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-message")
	req.Header.Set("Content-Type", "application/dns-message")

	resp, err := s.httpClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		// Fallback to GET method (base64 encoded)
		if resp != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				s.debugLog("Warning: failed to close response body: %v", closeErr)
			}
		}
		return tryDOHGet(s, url, buf)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.debugLog("Warning: failed to close response body: %v", closeErr)
		}
	}()

	return parseDOHResponse(resp)
}

// tryDOHGet attempts a GET request for DNS-over-HTTPS.
func tryDOHGet(s *DNSServer, url string, buf []byte) (*dns.Msg, error) {
	b64 := base64.RawURLEncoding.EncodeToString(buf)
	req, err := http.NewRequest("GET", url+"?dns="+b64, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Accept", "application/dns-message")
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			s.debugLog("Warning: failed to close response body: %v", closeErr)
		}
	}()
	return parseDOHResponse(resp)
}

// parseDOHResponse parses the DNS response from a DOH request.
func parseDOHResponse(resp *http.Response) (*dns.Msg, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	msg := new(dns.Msg)
	if err := msg.Unpack(body); err != nil {
		return nil, fmt.Errorf("failed to unpack DNS message: %w", err)
	}
	return msg, nil
}

// forwardRequest forwards the DNS request to upstream nameservers with request coalescing.
func (s *DNSServer) forwardRequest(w dns.ResponseWriter, r *dns.Msg, domain string, clientIP net.IP) {
	if len(s.nameservers) == 0 {
		s.sendErrorResponse(w, r, dns.RcodeServerFailure)
		return
	}

	// Double-check cache before forwarding (race condition protection)
	if cachedResp := s.getCachedResponse(r, clientIP); cachedResp != nil {
		if err := w.WriteMsg(cachedResp); err != nil {
			errorLog("Error writing cached response: %v", err)
		}
		return
	}

	// Get cache key for request coalescing
	key := getCacheKey(r)
	if key == "" {
		// Fallback to direct forwarding if we can't generate a key
		s.forwardDirect(w, r, domain)
		return
	}

	// Check if there's already a pending request for this key
	s.pendingMu.Lock()
	pending, exists := s.pendingRequests[key]
	if !exists {
		// Create new pending request and forward
		pending = &PendingRequest{
			waiters: make([]chan *dns.Msg, 0),
		}
		s.pendingRequests[key] = pending
		s.pendingMu.Unlock()
		s.handleFirstRequest(w, r, domain, key, pending)
		return
	}

	// There's already a pending request - wait for it
	s.pendingMu.Unlock()
	s.waitForPendingRequest(w, r, pending)
}

// handleFirstRequest handles the first request for a cache key.
func (s *DNSServer) handleFirstRequest(w dns.ResponseWriter, r *dns.Msg, domain, key string, pending *PendingRequest) {
	// Double-check cache before forwarding (in case it was just cached)
	if cachedResp := s.getCachedResponse(r, nil); cachedResp != nil {
		// Get waiters and clear them
		pending.mu.Lock()
		waiters := pending.waiters
		pending.waiters = nil
		pending.mu.Unlock()

		// Send cached response
		s.sendResponse(w, r, cachedResp)
		s.notifyWaiters(waiters, cachedResp, r)

		// Clean up pending request
		s.pendingMu.Lock()
		delete(s.pendingRequests, key)
		s.pendingMu.Unlock()
		return
	}

	// This is the first request - forward it
	resp := s.forwardDirectInternal(r, domain)

	// If request failed or timed out, create NXDOMAIN response and cache it
	if resp == nil {
		resp = s.createNXDOMAINResponse(r)
		// Cache the NXDOMAIN response
		if resp != nil {
			s.setCachedResponse(r, resp)
		}
	} else {
		// Log negative response types
		if isNegativeResponse(resp) {
			logNegativeResponse(s, resp, domain)
		}
		// Cache the response (including negative responses from upstream)
		s.setCachedResponse(r, resp)
	}

	// Get waiters and clear them
	pending.mu.Lock()
	waiters := pending.waiters
	pending.waiters = nil
	pending.mu.Unlock()

	// Send response to this request
	s.sendResponse(w, r, resp)

	// Notify all waiting requests
	s.notifyWaiters(waiters, resp, r)

	// Clean up pending request
	s.pendingMu.Lock()
	delete(s.pendingRequests, key)
	s.pendingMu.Unlock()
}

// waitForPendingRequest waits for a pending request to complete.
func (s *DNSServer) waitForPendingRequest(w dns.ResponseWriter, r *dns.Msg, pending *PendingRequest) {
	// Create a channel to wait for the response
	responseChan := make(chan *dns.Msg, 1)
	pending.mu.Lock()
	pending.waiters = append(pending.waiters, responseChan)
	pending.mu.Unlock()

	// Wait for response with timeout
	select {
	case resp := <-responseChan:
		s.sendResponse(w, r, resp)
	case <-time.After(10 * time.Second):
		// Timeout - check cache first (maybe it was cached while we waited)
		if cachedResp := s.getCachedResponse(r, nil); cachedResp != nil {
			s.sendResponse(w, r, cachedResp)
			return
		}
		// Create and cache NXDOMAIN response
		resp := s.createNXDOMAINResponse(r)
		if resp != nil {
			s.setCachedResponse(r, resp)
			s.sendResponse(w, r, resp)
		} else {
			s.sendErrorResponse(w, r, dns.RcodeServerFailure)
		}
	}
}

// notifyWaiters notifies all waiting requests of the response.
func (s *DNSServer) notifyWaiters(waiters []chan *dns.Msg, resp *dns.Msg, r *dns.Msg) {
	for _, waiter := range waiters {
		if resp != nil {
			// Create a copy for each waiter
			respCopy := resp.Copy()
			respCopy.Id = r.Id
			respCopy.Question = r.Question
			select {
			case waiter <- respCopy:
			default:
			}
		} else {
			close(waiter)
		}
	}
}

// sendResponse sends a DNS response to the client.
func (s *DNSServer) sendResponse(w dns.ResponseWriter, r *dns.Msg, resp *dns.Msg) {
	if resp != nil {
		// Update response ID to match this request
		resp.Id = r.Id
		resp.Question = r.Question
		if err := w.WriteMsg(resp); err != nil {
			errorLog("Error writing response: %v", err)
		}
	} else {
		s.sendErrorResponse(w, r, dns.RcodeServerFailure)
	}
}

// sendErrorResponse sends an error response to the client.
// nolint:unparam // rcode parameter kept for future extensibility
func (s *DNSServer) sendErrorResponse(w dns.ResponseWriter, r *dns.Msg, rcode int) {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.SetRcode(r, rcode)
	if err := w.WriteMsg(msg); err != nil {
		errorLog("Error writing response: %v", err)
	}
}

// forwardDirect forwards a request directly without coalescing (fallback).
func (s *DNSServer) forwardDirect(w dns.ResponseWriter, r *dns.Msg, domain string) {
	resp := s.forwardDirectInternal(r, domain)
	if resp == nil {
		// Request failed - create and cache NXDOMAIN response
		resp = s.createNXDOMAINResponse(r)
		if resp != nil {
			s.setCachedResponse(r, resp)
		}
	} else {
		s.setCachedResponse(r, resp)
	}

	if resp != nil {
		if err := w.WriteMsg(resp); err != nil {
			errorLog("Error writing response: %v", err)
		}
	} else {
		s.sendErrorResponse(w, r, dns.RcodeServerFailure)
	}
}

// forwardDirectInternal performs the actual forwarding and returns the response.
// Uses round-robin to distribute load across nameservers.
func (s *DNSServer) forwardDirectInternal(r *dns.Msg, domain string) *dns.Msg {
	if len(s.nameservers) == 0 {
		s.debugLog("No nameservers configured for %s", domain)
		return nil
	}

	// Get starting index using round-robin (atomic increment)
	// Safe conversion: number of nameservers is always small (< 1000)
	nsCount := uint64(len(s.nameservers))
	idxValue := atomic.AddUint64(&s.nameserverIdx, 1) - 1
	modValue := idxValue % nsCount
	// nolint:gosec // Safe: modValue is always < len(s.nameservers) which is small
	startIdx := int(modValue)

	// Try nameservers starting from the round-robin index, wrapping around
	for i := 0; i < len(s.nameservers); i++ {
		idx := (startIdx + i) % len(s.nameservers)
		nameserver := s.nameservers[idx]
		resp := s.tryForwardToNameserver(r, nameserver, domain)
		if resp != nil {
			return resp
		}
	}

	// All nameservers failed
	s.debugLog("All nameservers failed for %s, will return NXDOMAIN", domain)
	return nil
}

// tryForwardToNameserver attempts to forward a request to a specific nameserver.
func (s *DNSServer) tryForwardToNameserver(r *dns.Msg, nameserver NameserverConfig, domain string) *dns.Msg {
	address := net.JoinHostPort(nameserver.Address, fmt.Sprintf("%d", nameserver.Port))
	resp, err := s.forwardToNameserver(r, nameserver, address)
	if err != nil {
		s.debugLog("Error forwarding to %s (%s): %v", address, nameserver.Protocol, err)
		return nil
	}

	// Validate response matches query
	if resp != nil && !validateResponse(r, resp) {
		s.debugLog("Response validation failed for %s from %s, trying next nameserver", domain, address)
		return nil
	}

	// Handle truncated UDP responses - retry with TCP
	if resp != nil && resp.Truncated && !isTCPBasedProtocol(nameserver.Protocol) {
		resp = s.handleTruncatedResponse(r, address, domain)
	}

	// Log response type
	if resp != nil {
		s.logForwardedResponse(domain, address, nameserver.Protocol, resp)
	}
	return resp
}

// forwardToNameserver forwards a DNS request using the appropriate protocol.
func (s *DNSServer) forwardToNameserver(r *dns.Msg, nameserver NameserverConfig, address string) (*dns.Msg, error) {
	switch nameserver.Protocol {
	case protocolDOH:
		return s.forwardDOH(r, nameserver)
	case protocolDOT:
		return s.forwardDOT(r, address, nameserver.Address)
	case protocolTCP:
		tcpClient := &dns.Client{Net: protocolTCP, Timeout: 5 * time.Second}
		resp, _, err := tcpClient.Exchange(r, address)
		return resp, err
	default:
		// UDP DNS (default)
		resp, _, err := s.client.Exchange(r, address)
		return resp, err
	}
}

// forwardDOT forwards a DNS request using DNS-over-TLS.
func (s *DNSServer) forwardDOT(r *dns.Msg, address, serverName string) (*dns.Msg, error) {
	dotClient := &dns.Client{
		Net:     "tcp-tls",
		Timeout: 5 * time.Second,
		TLSConfig: &tls.Config{
			ServerName:         serverName,
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS12,
		},
	}
	resp, _, err := dotClient.Exchange(r, address)
	return resp, err
}

// isTCPBasedProtocol checks if a protocol uses TCP.
func isTCPBasedProtocol(protocol string) bool {
	return protocol == protocolTCP || protocol == protocolDOT || protocol == protocolDOH
}

// handleTruncatedResponse handles truncated UDP responses by retrying with TCP.
func (s *DNSServer) handleTruncatedResponse(r *dns.Msg, address, domain string) *dns.Msg {
	s.debugLog("Truncated UDP response for %s, retrying with TCP", domain)
	tcpClient := &dns.Client{Net: protocolTCP, Timeout: 5 * time.Second}
	tcpResp, _, tcpErr := tcpClient.Exchange(r, address)
	if tcpErr == nil && tcpResp != nil && validateResponse(r, tcpResp) {
		s.debugLog("Forwarded: %s -> %s (tcp, retry after truncation)", domain, address)
		return tcpResp
	}
	s.debugLog("TCP retry failed for %s: %v", domain, tcpErr)
	return nil
}

// logForwardedResponse logs a forwarded response with appropriate detail.
func (s *DNSServer) logForwardedResponse(domain, address, protocol string, resp *dns.Msg) {
	switch {
	case resp.Rcode == dns.RcodeNameError:
		s.debugLog("Forwarded: %s -> %s (%s) - NXDOMAIN", domain, address, protocol)
	case isNegativeResponse(resp):
		s.debugLog("Forwarded: %s -> %s (%s) - %s", domain, address, protocol, getRcodeName(resp.Rcode))
	default:
		s.debugLog("Forwarded: %s -> %s (%s)", domain, address, protocol)
	}
}

// logNegativeResponse logs when a negative response is received from upstream.
func logNegativeResponse(s *DNSServer, resp *dns.Msg, domain string) {
	switch resp.Rcode {
	case dns.RcodeNameError:
		s.debugLog("Received NXDOMAIN for %s from upstream, will cache", domain)
	case dns.RcodeServerFailure:
		s.debugLog("Received SERVFAIL for %s from upstream, will cache", domain)
	case dns.RcodeRefused:
		s.debugLog("Received REFUSED for %s from upstream, will cache", domain)
	case dns.RcodeNotImplemented:
		s.debugLog("Received NOTIMP for %s from upstream, will cache", domain)
	case dns.RcodeSuccess:
		if len(resp.Answer) == 0 {
			s.debugLog("Received NOERROR (no answers) for %s from upstream, will cache", domain)
		}
	}
}

// getRcodeName returns a human-readable name for a DNS response code.
func getRcodeName(rcode int) string {
	switch rcode {
	case dns.RcodeSuccess:
		return "NOERROR"
	case dns.RcodeFormatError:
		return "FORMERR"
	case dns.RcodeServerFailure:
		return "SERVFAIL"
	case dns.RcodeNameError:
		return "NXDOMAIN"
	case dns.RcodeNotImplemented:
		return "NOTIMP"
	case dns.RcodeRefused:
		return "REFUSED"
	default:
		return fmt.Sprintf("RCODE%d", rcode)
	}
}

// createNXDOMAINResponse creates an NXDOMAIN response for a failed query.
func (s *DNSServer) createNXDOMAINResponse(r *dns.Msg) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetReply(r)
	msg.Authoritative = true
	msg.SetRcode(r, dns.RcodeNameError)
	return msg
}
