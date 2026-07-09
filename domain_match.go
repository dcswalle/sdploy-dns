package main

import "strings"

// domainMatchesSuffix reports whether domain is suffix or a subdomain of suffix.
func domainMatchesSuffix(domain, suffix string) bool {
	return domain == suffix || strings.HasSuffix(domain, "."+suffix)
}

// matchOneLabelWildcard reports whether domain matches *.suffix (one label prefix).
// For example, *.tracker.com matches ads.tracker.com but not tracker.com.
func matchOneLabelWildcard(domain, suffix string) bool {
	if !strings.HasSuffix(domain, "."+suffix) {
		return false
	}
	prefix := domain[:len(domain)-len(suffix)-1]
	return prefix != "" && !strings.Contains(prefix, ".")
}
