package main

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func runNft(args ...string) error {
	cmd := exec.Command("nft", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nft %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}
	return nil
}

func runNftStdin(input string) error {
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("nft -f -: %w: %s", err, stderr.String())
	}
	return nil
}

// InitNftables creates (or recreates) the nft_blocker table with all sets and the forward chain.
func InitNftables(cfg *Config) error {
	// Delete table first (ignore error if it doesn't exist)
	_ = runNft("delete", "table", "inet", "nft_blocker")

	var sb strings.Builder
	sb.WriteString("table inet nft_blocker {\n")

	// Set for block-all by interface name
	sb.WriteString("    set blocked_ifaces {\n")
	sb.WriteString("        type ifname\n")
	sb.WriteString("    }\n\n")

	// One set per group
	for name := range cfg.Groups {
		fmt.Fprintf(&sb, "    set group_%s {\n", name)
		sb.WriteString("        type ether_addr\n")
		sb.WriteString("    }\n\n")
	}

	// Forward chain with rules referencing the sets
	sb.WriteString("    chain forward {\n")
	sb.WriteString("        type filter hook forward priority 0; policy accept;\n\n")
	sb.WriteString("        iifname @blocked_ifaces counter drop\n")
	for name := range cfg.Groups {
		fmt.Fprintf(&sb, "        ether saddr @group_%s counter drop\n", name)
	}
	sb.WriteString("    }\n")
	sb.WriteString("}\n")

	ruleset := sb.String()
	log.Printf("Initializing nftables:\n%s", ruleset)
	return runNftStdin(ruleset)
}

// BlockGroup adds all MAC addresses for a group to its named set.
func BlockGroup(groupName string, macs []string) error {
	if len(macs) == 0 {
		return nil
	}
	elements := strings.Join(macs, ", ")
	return runNft("add", "element", "inet", "nft_blocker",
		fmt.Sprintf("group_%s", groupName),
		fmt.Sprintf("{ %s }", elements))
}

// UnblockGroup flushes the named set for a group (removes all MAC elements).
func UnblockGroup(groupName string) error {
	return runNft("flush", "set", "inet", "nft_blocker", fmt.Sprintf("group_%s", groupName))
}

// BlockAllTraffic adds the configured interfaces to the blocked_ifaces set.
func BlockAllTraffic(ifaces []string) error {
	if len(ifaces) == 0 {
		return nil
	}
	quoted := make([]string, len(ifaces))
	for i, iface := range ifaces {
		quoted[i] = fmt.Sprintf("%q", iface)
	}
	return runNft("add", "element", "inet", "nft_blocker", "blocked_ifaces",
		fmt.Sprintf("{ %s }", strings.Join(quoted, ", ")))
}

// UnblockAllTraffic flushes the blocked_ifaces set.
func UnblockAllTraffic() error {
	return runNft("flush", "set", "inet", "nft_blocker", "blocked_ifaces")
}
