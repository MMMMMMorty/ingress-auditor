package utils

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck
)

// ConfigureCoreDNS patches the CoreDNS ConfigMap to add custom host entries.
// This allows controller to resolve test hostnames
// without requiring sudo access to modify /etc/hosts on the host machine.
func ConfigureCoreDNS(hostMappings map[string]string) error {
	if len(hostMappings) == 0 {
		return nil
	}

	// Build the hosts block for CoreDNS Corefile
	// Sort keys for deterministic output
	hostnames := make([]string, 0, len(hostMappings))
	for hostname := range hostMappings {
		hostnames = append(hostnames, hostname)
	}
	sort.Strings(hostnames)

	hostsEntries := make([]string, 0, len(hostnames))
	for _, hostname := range hostnames {
		ip := hostMappings[hostname]
		hostsEntries = append(hostsEntries, fmt.Sprintf("        %s %s", ip, hostname))
	}
	hostsBlock := strings.Join(hostsEntries, "\n")

	// The CoreDNS ConfigMap YAML with custom hosts block
	configMapYAML := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health {
           lameduck 5s
        }
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        hosts {
%s
            fallthrough
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
`, hostsBlock)

	// Write to temp file and apply
	tmpFile, err := os.CreateTemp("", "coredns-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			fmt.Printf("failed to remove temp file: %v", err)
		}
	}()

	if _, err := tmpFile.WriteString(configMapYAML); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	err = tmpFile.Close()
	if err != nil {
		fmt.Printf("failed to close tmpFIle: %v", err)
	}

	By("applying CoreDNS ConfigMap with custom host entries")
	cmd := exec.Command("kubectl", "apply", "-f", tmpFile.Name())
	_, err = Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to apply CoreDNS ConfigMap: %w", err)
	}

	By("restarting CoreDNS pods to apply configuration")
	cmd = exec.Command("kubectl", "rollout", "restart", "deployment/coredns", "-n", "kube-system")
	_, err = Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to restart CoreDNS: %w", err)
	}

	By("waiting for CoreDNS rollout to complete")
	cmd = exec.Command("kubectl", "rollout", "status", "deployment/coredns",
		"-n", "kube-system", "--timeout=120s")
	_, err = Run(cmd)
	if err != nil {
		return fmt.Errorf("CoreDNS rollout failed: %w", err)
	}

	// Give DNS a moment
	time.Sleep(5 * time.Second)

	return nil
}

func ResetCoreDNS() error {
	// The CoreDNS ConfigMap YAML with original setting
	configMapYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
data:
  Corefile: |
    .:53 {
        errors
        health {
           lameduck 5s
        }
        ready
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        hosts {
            fallthrough
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
`

	// Write to temp file and apply
	tmpFile, err := os.CreateTemp("", "coredns-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() {
		if err := os.Remove(tmpFile.Name()); err != nil {
			fmt.Printf("failed to remove temp file: %v", err)
		}
	}()

	if _, err := tmpFile.WriteString(configMapYAML); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	err = tmpFile.Close()
	if err != nil {
		fmt.Printf("failed to close tmpFIle: %v", err)
	}

	By("applying CoreDNS ConfigMap with custom host entries")
	cmd := exec.Command("kubectl", "apply", "-f", tmpFile.Name())
	_, err = Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to apply CoreDNS ConfigMap: %w", err)
	}

	By("restarting CoreDNS pods to apply configuration")
	cmd = exec.Command("kubectl", "rollout", "restart", "deployment/coredns", "-n", "kube-system")
	_, err = Run(cmd)
	if err != nil {
		return fmt.Errorf("failed to restart CoreDNS: %w", err)
	}

	By("waiting for CoreDNS rollout to complete")
	cmd = exec.Command("kubectl", "rollout", "status", "deployment/coredns",
		"-n", "kube-system", "--timeout=120s")
	_, err = Run(cmd)
	if err != nil {
		return fmt.Errorf("CoreDNS rollout failed: %w", err)
	}

	// Give DNS a moment
	time.Sleep(5 * time.Second)

	return nil
}
