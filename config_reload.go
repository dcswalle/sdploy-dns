package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// ApplyConfigDefaults fills in standard defaults for a parsed Config (same rules as startup).
func ApplyConfigDefaults(config *Config) {
	if config.ListenAddr == "" {
		config.ListenAddr = ":53"
	}
	if config.Nameservers == nil {
		config.Nameservers = []string{"8.8.8.8", "8.8.4.4"}
	}
}

// ReloadConfigFile re-reads the YAML file and applies it to the running server.
func (s *DNSServer) ReloadConfigFile(configPath string) error {
	s.reloadMu.Lock()
	defer s.reloadMu.Unlock()
	return s.reloadConfigFileUnlocked(configPath)
}

func (s *DNSServer) reloadConfigFileUnlocked(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	var newCfg Config
	if err := yaml.Unmarshal(data, &newCfg); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	ApplyEnvOverrides(&newCfg)
	ApplyConfigDefaults(&newCfg)

	prev := s.cfg()
	if prev != nil && newCfg.ListenAddr != prev.ListenAddr {
		log.Printf("config reload: keeping listen_addr %q (file has %q; changing bind address requires a restart)", prev.ListenAddr, newCfg.ListenAddr)
		newCfg.ListenAddr = prev.ListenAddr
	}

	if prev != nil {
		newCfg.Role = prev.Role
		newCfg.Master = prev.Master
		newCfg.Slave = prev.Slave
	}

	nameservers, err := parseNameservers(newCfg.Nameservers)
	if err != nil {
		return fmt.Errorf("parse nameservers: %w", err)
	}

	overwrites, err := parseOverwrites(newCfg.Overwrites)
	if err != nil {
		return fmt.Errorf("parse overwrites: %w", err)
	}

	temp := createDNSServerInstance(&newCfg, nameservers, overwrites)
	if err := temp.loadBlockLists(); err != nil {
		return fmt.Errorf("load block lists: %w", err)
	}

	s.stopBlockListReloader()

	if newCfg.GOGC > 0 {
		debug.SetGCPercent(newCfg.GOGC)
		log.Printf("GOGC set to %d%%", newCfg.GOGC)
	}

	s.clearDNSCache()

	s.mu.Lock()
	s.setConfig(&newCfg)
	s.nameservers = temp.nameservers
	s.overwrites = temp.overwrites
	s.blocked = temp.blocked
	s.urlBlockLists = temp.urlBlockLists
	s.httpClient = temp.httpClient
	s.maxCacheSize = temp.maxCacheSize
	s.mu.Unlock()

	if len(s.urlBlockLists) > 0 && newCfg.ReloadInterval > 0 {
		s.startBlockListReloader(time.Duration(newCfg.ReloadInterval) * time.Minute)
		log.Printf("URL-based block list reloader started (interval: %d minutes)", newCfg.ReloadInterval)
	}

	log.Printf("Config reloaded from %s (%d blocked hosts, %d overwrites, %d nameservers)",
		configPath, len(s.blocked), len(s.overwrites), len(s.nameservers))
	return nil
}

// StartConfigWatcher watches the config file (and directory renames) and reloads when it changes.
func (s *DNSServer) StartConfigWatcher(configPath string) error {
	abs, err := filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fsnotify: %w", err)
	}

	dir := filepath.Dir(abs)
	base := filepath.Base(abs)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("watch %s: %w", dir, err)
	}

	go s.runConfigWatcher(watcher, abs, base)
	log.Printf("Watching %s for configuration changes", abs)
	return nil
}

func (s *DNSServer) runConfigWatcher(watcher *fsnotify.Watcher, absPath, baseName string) {
	defer func() { _ = watcher.Close() }()

	var debounce *time.Timer
	debounceReload := func() {
		if debounce != nil {
			debounce.Stop()
		}
		debounce = time.AfterFunc(300*time.Millisecond, func() {
			if err := s.ReloadConfigFile(absPath); err != nil {
				log.Printf("Config reload failed: %v", err)
			}
		})
	}

	for {
		select {
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			if filepath.Base(ev.Name) != baseName {
				continue
			}
			if ev.Has(fsnotify.Write) || ev.Has(fsnotify.Create) || ev.Has(fsnotify.Rename) {
				debounceReload()
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Config watcher error: %v", err)
		}
	}
}
