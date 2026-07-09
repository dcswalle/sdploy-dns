package main

import "testing"

func TestDomainMatchesSuffix(t *testing.T) {
	tests := []struct {
		domain string
		suffix string
		want   bool
	}{
		{"example.com", "example.com", true},
		{"www.example.com", "example.com", true},
		{"a.b.example.com", "example.com", true},
		{"notexample.com", "example.com", false},
		{"example.com.evil.com", "example.com", false},
	}

	for _, tt := range tests {
		if got := domainMatchesSuffix(tt.domain, tt.suffix); got != tt.want {
			t.Errorf("domainMatchesSuffix(%q, %q) = %v, want %v", tt.domain, tt.suffix, got, tt.want)
		}
	}
}

func TestMatchOneLabelWildcard(t *testing.T) {
	tests := []struct {
		domain string
		suffix string
		want   bool
	}{
		{"ads.tracker.com", "tracker.com", true},
		{"tracker.com", "tracker.com", false},
		{"a.b.tracker.com", "tracker.com", false},
		{"ads.other.com", "tracker.com", false},
	}

	for _, tt := range tests {
		if got := matchOneLabelWildcard(tt.domain, tt.suffix); got != tt.want {
			t.Errorf("matchOneLabelWildcard(%q, %q) = %v, want %v", tt.domain, tt.suffix, got, tt.want)
		}
	}
}

func TestParseFilterRule(t *testing.T) {
	tests := []struct {
		line     string
		ok       bool
		kind     BlockRuleKind
		domain   string
		wildcard string
	}{
		{"||example.com^", true, BlockSuffix, "example.com", ""},
		{"127.0.0.1 ads.example.com", true, BlockSuffix, "ads.example.com", ""},
		{"malware-site.com", true, BlockSuffix, "malware-site.com", ""},
		{"||*.tracker.com^", true, BlockWildcard, "", "tracker.com"},
		{"*.tracker.com", true, BlockWildcard, "", "tracker.com"},
		{"@@||example.com^", false, 0, "", ""},
		{"! example.com", false, 0, "", ""},
		{"/ads/", false, 0, "", ""},
		{"https://example.com/ads", false, 0, "", ""},
		{"# comment", false, 0, "", ""},
	}

	for _, tt := range tests {
		rule, ok := parseFilterRule(tt.line)
		if ok != tt.ok {
			t.Errorf("parseFilterRule(%q) ok = %v, want %v", tt.line, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if rule.Kind != tt.kind {
			t.Errorf("parseFilterRule(%q) kind = %v, want %v", tt.line, rule.Kind, tt.kind)
		}
		if rule.Domain != tt.domain {
			t.Errorf("parseFilterRule(%q) domain = %q, want %q", tt.line, rule.Domain, tt.domain)
		}
		if rule.WildcardSuffix != tt.wildcard {
			t.Errorf("parseFilterRule(%q) wildcard = %q, want %q", tt.line, rule.WildcardSuffix, tt.wildcard)
		}
	}
}

func TestIsBlockedSuffixAndWildcard(t *testing.T) {
	s := &DNSServer{
		blockedSuffix: map[string]*BlockEntry{
			"example.com": {},
		},
		blockedWildcard: []WildcardBlock{
			{Suffix: "tracker.com", Entry: &BlockEntry{}},
		},
	}

	if !s.isBlocked("www.example.com", nil) {
		t.Fatal("expected www.example.com to be blocked by suffix rule")
	}
	if s.isBlocked("notexample.com", nil) {
		t.Fatal("did not expect notexample.com to be blocked")
	}
	if !s.isBlocked("ads.tracker.com", nil) {
		t.Fatal("expected ads.tracker.com to be blocked by wildcard rule")
	}
	if s.isBlocked("tracker.com", nil) {
		t.Fatal("did not expect bare tracker.com to match *.tracker.com")
	}
}

func TestParseOverwritesWildcard(t *testing.T) {
	index, err := parseOverwrites(map[string]interface{}{
		"*.local.sdploy.com": "10.0.0.5",
		"myserver.local.sdploy.com": "192.168.1.10",
	})
	if err != nil {
		t.Fatalf("parseOverwrites: %v", err)
	}
	if len(index.Wildcards) != 1 || index.Wildcards[0].Suffix != "local.sdploy.com" {
		t.Fatalf("unexpected wildcards: %+v", index.Wildcards)
	}
	if index.Exact["myserver.local.sdploy.com"].IP != "192.168.1.10" {
		t.Fatalf("unexpected exact overwrite: %+v", index.Exact)
	}
}

func TestGetOverwriteWildcardPriority(t *testing.T) {
	index, err := parseOverwrites(map[string]interface{}{
		"*.local.sdploy.com":          "10.0.0.5",
		"myserver.local.sdploy.com":   "192.168.1.10",
	})
	if err != nil {
		t.Fatalf("parseOverwrites: %v", err)
	}

	s := &DNSServer{overwrites: index}

	if ip, ok := s.getOverwrite("other.local.sdploy.com", nil); !ok || ip != "10.0.0.5" {
		t.Fatalf("wildcard overwrite = %q, ok=%v", ip, ok)
	}
	if ip, ok := s.getOverwrite("myserver.local.sdploy.com", nil); !ok || ip != "192.168.1.10" {
		t.Fatalf("exact overwrite should win: %q, ok=%v", ip, ok)
	}
}
