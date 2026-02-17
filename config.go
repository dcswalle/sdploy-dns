package main

import (
	"fmt"
	"net"
	"strings"
)

// parseNameserverFromString parses a simple string nameserver configuration.
func parseNameserverFromString(val string) NameserverConfig {
	ns := NameserverConfig{
		Address:  val,
		Protocol: protocolUDP,
		Port:     53,
	}
	// Check if it contains a port
	if strings.Contains(val, ":") {
		host, portStr, err := net.SplitHostPort(val)
		if err == nil {
			ns.Address = host
			if port, err := net.LookupPort("", portStr); err == nil {
				ns.Port = port
			}
		}
	}
	return ns
}

// parseNameserverFromMap parses a map-based nameserver configuration.
func parseNameserverFromMap(val map[string]interface{}) NameserverConfig {
	ns := NameserverConfig{
		Protocol: protocolUDP,
		Port:     53,
	}
	if addr, ok := val["address"].(string); ok {
		ns.Address = addr
	}
	if proto, ok := val["protocol"].(string); ok {
		ns.Protocol = strings.ToLower(proto)
	}
	if port, ok := val["port"].(int); ok {
		ns.Port = port
	} else if port, ok := val["port"].(string); ok {
		if p, err := net.LookupPort("", port); err == nil {
			ns.Port = p
		}
	}
	// Set default ports based on protocol
	if ns.Port == 53 {
		switch ns.Protocol {
		case protocolDOT:
			ns.Port = 853
		case protocolDOH:
			ns.Port = 443
		}
	}
	return ns
}

// parseNameserverFromMapInterface parses a map-based nameserver configuration (fallback format).
func parseNameserverFromMapInterface(val map[interface{}]interface{}) NameserverConfig {
	ns := NameserverConfig{
		Protocol: protocolUDP,
		Port:     53,
	}
	if addr, ok := val["address"].(string); ok {
		ns.Address = addr
	}
	if proto, ok := val["protocol"].(string); ok {
		ns.Protocol = strings.ToLower(proto)
	}
	if port, ok := val["port"].(int); ok {
		ns.Port = port
	} else if port, ok := val["port"].(string); ok {
		if p, err := net.LookupPort("", port); err == nil {
			ns.Port = p
		}
	}
	// Set default ports based on protocol
	if ns.Port == 53 {
		switch ns.Protocol {
		case protocolDOT:
			ns.Port = 853
		case protocolDOH:
			ns.Port = 443
		}
	}
	return ns
}

// parseNameservers parses nameserver configuration (supports both old and new format).
func parseNameservers(nameservers interface{}) ([]NameserverConfig, error) {
	var result []NameserverConfig

	switch v := nameservers.(type) {
	case []interface{}:
		for _, item := range v {
			switch val := item.(type) {
			case string:
				result = append(result, parseNameserverFromString(val))
			case map[string]interface{}:
				result = append(result, parseNameserverFromMap(val))
			case map[interface{}]interface{}:
				result = append(result, parseNameserverFromMapInterface(val))
			}
		}
	case []string:
		for _, addr := range v {
			result = append(result, parseNameserverFromString(addr))
		}
	default:
		return nil, fmt.Errorf("invalid nameservers format")
	}

	return result, nil
}

// parseOverwriteIPs parses IPs from an overwrite entry.
func parseOverwriteIPs(ips []interface{}, domain string) (string, []net.IP, error) {
	if len(ips) == 0 {
		return "", nil, fmt.Errorf("missing or empty 'ips' field for overwrite %s (at least one IP required)", domain)
	}
	firstIP, ok := ips[0].(string)
	if !ok {
		return "", nil, fmt.Errorf("first element in 'ips' must be a string for overwrite %s", domain)
	}
	var ipList []net.IP
	for _, ipStr := range ips {
		if s, ok := ipStr.(string); ok {
			ip := net.ParseIP(s)
			if ip != nil {
				ipList = append(ipList, ip)
			}
		}
	}
	return firstIP, ipList, nil
}

// parseOverwriteSubnets parses subnets from an overwrite entry.
func parseOverwriteSubnets(subnets []interface{}) ([]*net.IPNet, error) {
	var subnetList []*net.IPNet
	for _, subnetStr := range subnets {
		if s, ok := subnetStr.(string); ok {
			ipNet, err := parseSubnet(s)
			if err != nil {
				return nil, fmt.Errorf("invalid subnet %s: %w", s, err)
			}
			subnetList = append(subnetList, ipNet)
		}
	}
	return subnetList, nil
}

// parseOverwriteFromMap parses a map-based overwrite entry.
func parseOverwriteFromMap(v map[string]interface{}, domain string) (*OverwriteEntry, error) {
	entry := &OverwriteEntry{}
	if ips, ok := v["ips"].([]interface{}); ok {
		firstIP, ipList, err := parseOverwriteIPs(ips, domain)
		if err != nil {
			return nil, err
		}
		entry.IP = firstIP
		entry.IPs = ipList
	} else {
		return nil, fmt.Errorf("missing or empty 'ips' field for overwrite %s (at least one IP required)", domain)
	}
	if subnets, ok := v["subnets"].([]interface{}); ok {
		subnetList, err := parseOverwriteSubnets(subnets)
		if err != nil {
			return nil, err
		}
		entry.Subnets = subnetList
	}
	return entry, nil
}

// parseOverwriteFromMapInterface parses a map-based overwrite entry (fallback format).
func parseOverwriteFromMapInterface(v map[interface{}]interface{}, domain string) (*OverwriteEntry, error) {
	entry := &OverwriteEntry{}
	if ips, ok := v["ips"].([]interface{}); ok {
		firstIP, ipList, err := parseOverwriteIPs(ips, domain)
		if err != nil {
			return nil, err
		}
		entry.IP = firstIP
		entry.IPs = ipList
	} else {
		return nil, fmt.Errorf("missing or empty 'ips' field for overwrite %s (at least one IP required)", domain)
	}
	if subnets, ok := v["subnets"].([]interface{}); ok {
		subnetList, err := parseOverwriteSubnets(subnets)
		if err != nil {
			return nil, err
		}
		entry.Subnets = subnetList
	}
	return entry, nil
}

// parseOverwrites parses overwrite configuration (supports both old and new format).
func parseOverwrites(overwrites map[string]interface{}) (map[string]*OverwriteEntry, error) {
	result := make(map[string]*OverwriteEntry)

	for domain, value := range overwrites {
		// Skip comment entries (YAML parser might include them as keys with nil values)
		if value == nil {
			continue
		}

		entry := &OverwriteEntry{}

		switch v := value.(type) {
		case string:
			// Old format: simple IP string
			entry.IP = v
		case map[string]interface{}:
			var err error
			entry, err = parseOverwriteFromMap(v, domain)
			if err != nil {
				return nil, err
			}
		case map[interface{}]interface{}:
			var err error
			entry, err = parseOverwriteFromMapInterface(v, domain)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("invalid overwrite format for %s (got type %T, value: %v)", domain, value, value)
		}

		if entry.IP == "" {
			return nil, fmt.Errorf("missing IP for overwrite %s", domain)
		}

		result[normalizeDomain(domain)] = entry
	}

	return result, nil
}
