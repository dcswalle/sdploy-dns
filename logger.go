package main

import "log"

// debugLog logs a message only if debug mode is enabled.
func (s *DNSServer) debugLog(format string, v ...interface{}) {
	if c := s.cfg(); c != nil && c.Debug {
		log.Printf(format, v...)
	}
}

// logBlock logs a blocked request only if log_blocks is enabled.
func (s *DNSServer) logBlock(format string, v ...interface{}) {
	if c := s.cfg(); c != nil && c.LogBlocks {
		log.Printf(format, v...)
	}
}

// logOverwrite logs an overwritten request only if log_overwrites is enabled.
func (s *DNSServer) logOverwrite(format string, v ...interface{}) {
	if c := s.cfg(); c != nil && c.LogOverwrites {
		log.Printf(format, v...)
	}
}

// errorLog always logs errors regardless of debug mode.
func errorLog(format string, v ...interface{}) {
	log.Printf(format, v...)
}
