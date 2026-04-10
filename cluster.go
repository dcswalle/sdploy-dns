package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// --- Master ---

func (s *DNSServer) startMasterAPI(configPath string) error {
	cfg := s.cfg()
	if cfg.Master == nil {
		return fmt.Errorf("master config block is required when role=master")
	}

	addr := cfg.Master.APIAddr
	if addr == "" {
		addr = ":8053"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /config", s.authMiddleware(s.handleGetConfig(configPath)))
	mux.HandleFunc("GET /health", s.authMiddleware(s.handleHealth()))

	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("Master API listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Master API error: %v", err)
		}
	}()
	return nil
}

func (s *DNSServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := s.cfg()
		key := ""
		if cfg.Master != nil {
			key = cfg.Master.APIKey
		}
		if key != "" {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != key {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (s *DNSServer) handleGetConfig(configPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile(configPath)
		if err != nil {
			http.Error(w, `{"error":"failed to read config"}`, http.StatusInternalServerError)
			return
		}

		etag := fmt.Sprintf(`"%x"`, sha256.Sum256(data))

		if match := r.Header.Get("If-None-Match"); match == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set("ETag", etag)
		w.Write(data)
	}
}

func (s *DNSServer) handleHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"role":   "master",
		})
	}
}

// --- Slave ---

func (s *DNSServer) startSlaveSync(configPath string) {
	cfg := s.cfg()
	if cfg.Slave == nil {
		log.Printf("slave config block missing; sync disabled")
		return
	}

	interval := cfg.Slave.SyncInterval
	if interval <= 0 {
		interval = 30
	}

	sc := &slaveSyncer{
		server:     s,
		configPath: configPath,
		masterURL:  strings.TrimRight(cfg.Slave.MasterURL, "/"),
		apiKey:     cfg.Slave.APIKey,
		client:     &http.Client{Timeout: 15 * time.Second},
	}

	go func() {
		sc.syncOnce()

		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			sc.syncOnce()
		}
	}()

	log.Printf("Slave sync started (master=%s, interval=%ds)", sc.masterURL, interval)
}

type slaveSyncer struct {
	server     *DNSServer
	configPath string
	masterURL  string
	apiKey     string
	client     *http.Client

	mu      sync.Mutex
	lastTag string
}

func (sc *slaveSyncer) syncOnce() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	req, err := http.NewRequest("GET", sc.masterURL+"/config", nil)
	if err != nil {
		log.Printf("slave sync: build request: %v", err)
		return
	}
	if sc.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+sc.apiKey)
	}
	if sc.lastTag != "" {
		req.Header.Set("If-None-Match", sc.lastTag)
	}

	resp, err := sc.client.Do(req)
	if err != nil {
		log.Printf("slave sync: fetch config: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return
	}
	if resp.StatusCode == http.StatusUnauthorized {
		log.Printf("slave sync: unauthorized — check api_key")
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("slave sync: unexpected status %d", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("slave sync: read body: %v", err)
		return
	}

	merged, err := sc.mergeConfigs(body)
	if err != nil {
		log.Printf("slave sync: merge config: %v", err)
		return
	}

	current, _ := os.ReadFile(sc.configPath)
	if bytes.Equal(current, merged) {
		sc.lastTag = resp.Header.Get("ETag")
		return
	}

	if err := os.WriteFile(sc.configPath, merged, 0644); err != nil {
		log.Printf("slave sync: write config: %v", err)
		return
	}

	sc.lastTag = resp.Header.Get("ETag")
	log.Printf("slave sync: config updated from master")
}

// mergeConfigs takes the master's raw YAML and returns a merged YAML that
// preserves the slave's local fields (role, master, slave, listen_addr).
func (sc *slaveSyncer) mergeConfigs(masterYAML []byte) ([]byte, error) {
	var masterCfg Config
	if err := yaml.Unmarshal(masterYAML, &masterCfg); err != nil {
		return nil, fmt.Errorf("parse master config: %w", err)
	}

	localCfg := sc.server.cfg()
	if localCfg == nil {
		return nil, fmt.Errorf("local config not available")
	}

	masterCfg.ListenAddr = localCfg.ListenAddr
	masterCfg.Role = localCfg.Role
	masterCfg.Master = localCfg.Master
	masterCfg.Slave = localCfg.Slave

	out, err := yaml.Marshal(&masterCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal merged config: %w", err)
	}
	return out, nil
}
