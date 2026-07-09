package main

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

const envPrefix = "GO_DNS_"

// loadDotEnv reads a .env file and sets variables that are not already defined.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}

func envString(key string) (string, bool) {
	val, ok := os.LookupEnv(envPrefix + key)
	if !ok || val == "" {
		return "", false
	}
	return val, true
}

func envBool(key string) (bool, bool) {
	val, ok := envString(key)
	if !ok {
		return false, false
	}
	parsed, err := strconv.ParseBool(val)
	if err != nil {
		return false, false
	}
	return parsed, true
}

func envInt(key string) (int, bool) {
	val, ok := envString(key)
	if !ok {
		return 0, false
	}
	parsed, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

// ApplyEnvOverrides applies GO_DNS_* environment variables on top of YAML config.
func ApplyEnvOverrides(config *Config) {
	if val, ok := envString("LISTEN_ADDR"); ok {
		config.ListenAddr = val
	}
	if val, ok := envString("ROLE"); ok {
		config.Role = val
	}
	if val, ok := envString("MASTER_API_ADDR"); ok {
		if config.Master == nil {
			config.Master = &MasterConfig{}
		}
		config.Master.APIAddr = val
	}
	if val, ok := envString("MASTER_API_KEY"); ok {
		if config.Master == nil {
			config.Master = &MasterConfig{}
		}
		config.Master.APIKey = val
	}
	if val, ok := envString("SLAVE_MASTER_URL"); ok {
		if config.Slave == nil {
			config.Slave = &SlaveConfig{}
		}
		config.Slave.MasterURL = val
	}
	if val, ok := envString("SLAVE_API_KEY"); ok {
		if config.Slave == nil {
			config.Slave = &SlaveConfig{}
		}
		config.Slave.APIKey = val
	}
	if val, ok := envInt("SLAVE_SYNC_INTERVAL"); ok {
		if config.Slave == nil {
			config.Slave = &SlaveConfig{}
		}
		config.Slave.SyncInterval = val
	}
	if val, ok := envString("NAMESERVERS"); ok {
		parts := strings.Split(val, ",")
		nameservers := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				nameservers = append(nameservers, part)
			}
		}
		if len(nameservers) > 0 {
			config.Nameservers = nameservers
		}
	}
	if val, ok := envInt("CACHE_TTL"); ok {
		config.CacheTTL = val
	}
	if val, ok := envInt("NEGATIVE_CACHE_TTL"); ok {
		config.NegativeCacheTTL = val
	}
	if val, ok := envInt("MAX_CACHE_SIZE"); ok {
		config.MaxCacheSize = val
	}
	if val, ok := envInt("RELOAD_INTERVAL"); ok {
		config.ReloadInterval = val
	}
	if val, ok := envString("FALLBACK_DNS"); ok {
		config.FallbackDNS = val
	}
	if val, ok := envString("DNS_CHECK_DOMAIN"); ok {
		config.DNSCheckDomain = val
	}
	if val, ok := envInt("GOGC"); ok {
		config.GOGC = val
	}
	if val, ok := envBool("DEBUG"); ok {
		config.Debug = val
	}
	if val, ok := envBool("LOG_BLOCKS"); ok {
		config.LogBlocks = val
	}
	if val, ok := envBool("LOG_OVERWRITES"); ok {
		config.LogOverwrites = val
	}
}

// ConfigFileFromEnv returns the config file path from GO_DNS_CONFIG_FILE.
func ConfigFileFromEnv() string {
	if val, ok := envString("CONFIG_FILE"); ok {
		return val
	}
	return ""
}
