// Package main provides a DNS server with blocking, overwriting, and forwarding capabilities.
// It supports DNS-over-TLS (DOT) and DNS-over-HTTPS (DOH), conditional blocking by IP/subnet,
// and DNS overwrites with IP/subnet restrictions.
package main

import (
	"log"
	"os"
	"runtime/debug"
	"strings"

	"github.com/miekg/dns"
	"gopkg.in/yaml.v3"
)

func main() {
	// Load configuration
	configFile := "config.yml"
	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}

	configData, err := os.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Failed to read config file %s: %v", configFile, err)
	}

	var config Config
	if err := yaml.Unmarshal(configData, &config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	ApplyConfigDefaults(&config)

	// Set GOGC if configured (tune garbage collection)
	if config.GOGC > 0 {
		debug.SetGCPercent(config.GOGC)
		log.Printf("GOGC set to %d%%", config.GOGC)
	}

	// Create and start DNS server
	server, err := NewDNSServer(&config)
	if err != nil {
		log.Fatalf("Failed to create DNS server: %v", err)
	}

	if err := server.StartConfigWatcher(configFile); err != nil {
		log.Printf("Config hot-reload disabled: %v", err)
	}

	switch strings.ToLower(config.Role) {
	case "master":
		if err := server.startMasterAPI(configFile); err != nil {
			log.Fatalf("Failed to start master API: %v", err)
		}
	case "slave":
		server.startSlaveSync(configFile)
	}

	// Start TCP server as well (for larger responses)
	go func() {
		tcpServer := &dns.Server{
			Addr:    config.ListenAddr,
			Net:     "tcp",
			Handler: dns.HandlerFunc(server.handleDNSRequest),
		}
		if err := tcpServer.ListenAndServe(); err != nil {
			errorLog("TCP server error: %v", err)
		}
	}()

	// Start UDP server (main)
	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start DNS server: %v", err)
	}
}
