package main

import (
	"net"
	"strings"
)

// BlockRuleKind describes how a block-list rule matches query names.
type BlockRuleKind int

const (
	BlockSuffix BlockRuleKind = iota
	BlockWildcard
)

// ParsedBlockRule is a normalized DNS-applicable block rule from a filter list line.
type ParsedBlockRule struct {
	Kind           BlockRuleKind
	Domain         string
	WildcardSuffix string
}

// parseFilterRule parses an adblock/AdGuard-style filter line into a DNS block rule.
// Returns false for comments, URL/path/regex rules, and allowlist exceptions.
func parseFilterRule(line string) (ParsedBlockRule, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ParsedBlockRule{}, false
	}

	// Allowlist / exception rules are not applied in this phase.
	if strings.HasPrefix(line, "@@") || strings.HasPrefix(line, "!") {
		return ParsedBlockRule{}, false
	}

	// Browser-only rules: regex, paths, full URLs.
	if strings.HasPrefix(line, "/") || strings.Contains(line, "://") {
		return ParsedBlockRule{}, false
	}

	raw := line

	// Hosts format: 127.0.0.1 domain or 0.0.0.0 domain
	if parts := strings.Fields(raw); len(parts) >= 2 && net.ParseIP(parts[0]) != nil {
		raw = parts[1]
	}

	domain := raw
	domain = strings.TrimPrefix(domain, "||")
	if i := strings.Index(domain, "$"); i >= 0 {
		domain = domain[:i]
	}
	domain = strings.TrimSuffix(domain, "^")
	domain = strings.TrimSuffix(domain, "/")
	domain = strings.TrimSpace(domain)
	domain = normalizeDomain(domain)

	if domain == "" || strings.Contains(domain, "/") {
		return ParsedBlockRule{}, false
	}

	if strings.HasPrefix(domain, "*.") {
		suffix := strings.TrimPrefix(domain, "*.")
		if suffix == "" {
			return ParsedBlockRule{}, false
		}
		return ParsedBlockRule{
			Kind:           BlockWildcard,
			WildcardSuffix: suffix,
		}, true
	}

	return ParsedBlockRule{
		Kind:   BlockSuffix,
		Domain: domain,
	}, true
}
