package main

import (
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Loaded config: %d groups, listen=%s, interface=%s", len(cfg.Groups), cfg.Listen, cfg.Interface)

	// Load persisted state
	state := NewState(cfg.StateFile)
	if err := state.Load(); err != nil {
		log.Fatalf("Failed to load state: %v", err)
	}

	// Initialize nftables table and sets
	if err := InitNftables(cfg); err != nil {
		log.Fatalf("Failed to initialize nftables: %v", err)
	}
	log.Println("nftables table initialized")

	// Timer manager for timed blocks
	timers := NewTimerManager()

	// Create server (needed for timer callbacks)
	srv := NewServer(cfg, state, timers)

	// Restore state from previous run
	snap := state.Snapshot()

	// Restore block-all
	if snap.BlockAll {
		if err := BlockAllTraffic(cfg.Interface); err != nil {
			log.Printf("WARNING: failed to restore block-all: %v", err)
		} else {
			log.Println("Restored: block-all is active")
		}
	}

	// Restore per-group blocks
	for name, gs := range snap.Groups {
		if !gs.Blocked {
			continue
		}
		group, ok := cfg.Groups[name]
		if !ok {
			log.Printf("WARNING: state references unknown group %q, skipping", name)
			continue
		}
		if err := BlockGroup(name, group.MACAddresses); err != nil {
			log.Printf("WARNING: failed to restore block for group %s: %v", name, err)
			continue
		}
		log.Printf("Restored: group %s is blocked", name)

		// If timed, schedule auto-unblock
		if gs.BlockedUntil != nil && !gs.BlockedUntil.IsZero() {
			remaining := time.Until(*gs.BlockedUntil)
			if remaining <= 0 {
				// Already expired, unblock now
				log.Printf("Timer for group %s already expired, unblocking", name)
				if err := UnblockGroup(name); err != nil {
					log.Printf("WARNING: failed to unblock expired group %s: %v", name, err)
				}
				state.SetGroupBlocked(name, false, nil)
				_ = state.Save()
			} else {
				groupName := name
				timers.Start(groupName, remaining, func() {
					log.Printf("Timer expired for group %s, unblocking", groupName)
					if err := UnblockGroup(groupName); err != nil {
						log.Printf("ERROR auto-unblocking group %s: %v", groupName, err)
					}
					srv.state.SetGroupBlocked(groupName, false, nil)
					if err := srv.state.Save(); err != nil {
						log.Printf("ERROR saving state: %v", err)
					}
				})
				log.Printf("Restored timer for group %s: %v remaining", name, remaining.Round(time.Second))
			}
		}
	}

	// Start HTTP server
	log.Printf("Starting HTTP server on %s", cfg.Listen)
	if err := http.ListenAndServe(cfg.Listen, srv.Handler()); err != nil {
		log.Fatalf("HTTP server failed: %v", err)
	}
}
