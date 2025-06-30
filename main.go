package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

type CheckResult struct {
	address string
	success bool
	message string
}

func main() {
	fileName := "data.csv"
	file, err := os.Open(fileName)
	if err != nil {
		log.Println("Failed to open file: ", fileName)
		log.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			panic(err)
		}
	}()

	r := csv.NewReader(file)

	records, err := r.ReadAll()
	if err != nil {
		log.Println("Failed to read file: ", fileName)
		log.Fatal(err)
	}

	results := make(chan CheckResult)
	var wg sync.WaitGroup

	for _, record := range records[1:] { // Skip header row

		destIp := record[0]
		port := record[1]

		if port == "" {
			port = "443"
		}
		var portRange []string
		if strings.Contains(port, "-") {
			portRange = strings.Split(port, "-")
			for _, p := range portRange {
				wg.Add(1)
				go func(ip, port string) {
					defer wg.Done()
					processTarget(ip, p, results)
				}(destIp, port)
			}
		} else {
			wg.Add(1)
			go func(ip, port string) {
				defer wg.Done()
				processTarget(ip, port, results)
			}(destIp, port)
		}

	}
	// Launch a goroutine to close results channel after all checks complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect and print results as they come in
	for result := range results {
		fmt.Println(result.message)
	}
}

func parseEndpoint(ip string) bool {
	_, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return true
}

func dialEndpointAsync(ip string, port string, results chan<- CheckResult) {
	address := net.JoinHostPort(ip, port)
	isIPv6 := strings.Contains(ip, ":")

	var dialNetwork string
	if isIPv6 {
		dialNetwork = "tcp6"
	} else {
		dialNetwork = "tcp4"
	}

	conn, err := net.DialTimeout(dialNetwork, address, time.Second*3)
	if err != nil {
		var message string

		// More specific error checking
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			message = fmt.Sprintf("⏳ Connection timeout to %s", address)
		} else if opErr, ok := err.(*net.OpError); ok {
			if opErr.Timeout() {
				message = fmt.Sprintf("⏳ Connection timeout to %s", address)
			} else if syscallErr, ok := opErr.Err.(*os.SyscallError); ok {
				if syscallErr.Err == syscall.ECONNREFUSED {
					message = fmt.Sprintf("🛑 Connection refused to %s", address)
				} else if syscallErr.Err == syscall.ENETUNREACH {
					message = fmt.Sprintf("🤷🏼‍♂️ Network unreachable for %s", address)
				} else {
					message = fmt.Sprintf("System error for %s: %v", address, syscallErr.Err)
				}
			} else {
				message = fmt.Sprintf("Operation error for %s: %v", address, opErr.Err)
			}
		} else {
			message = fmt.Sprintf("Failed to connect to %s: %v", address, err)
		}

		results <- CheckResult{address: address, success: false, message: message}
		return
	}

	defer conn.Close()
	results <- CheckResult{
		address: address,
		success: true,
		message: fmt.Sprintf("✅ Connected to %s", address),
	}

}

// expandTarget processes a target (IP, CIDR, or FQDN) and returns a list of IPs to check
func expandTarget(target string) ([]string, error) {
	// Check if it's a CIDR range
	if strings.Contains(target, "/") {
		return expandCIDR(target)
	}

	// Try to parse as IP address
	if ip := net.ParseIP(target); ip != nil {
		return []string{target}, nil
	}

	// Assume it's a FQDN, try to resolve it
	ips, err := net.LookupHost(target)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve %s: %v", target, err)
	}
	return ips, nil
}

// expandCIDR takes a CIDR range and returns all IP addresses in that range
func expandCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR range %s: %v", cidr, err)
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); incrementIP(ip) {
		ips = append(ips, ip.String())
	}

	// Remove network address and broadcast address for IPv4
	if len(ips) > 2 && !strings.Contains(cidr, "::") {
		ips = ips[1 : len(ips)-1]
	}

	return ips, nil
}

// incrementIP increments an IP address by 1
func incrementIP(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// Modified main processing function
func processTarget(target string, port string, results chan<- CheckResult) {
	ips, err := expandTarget(target)
	if err != nil {
		results <- CheckResult{
			address: fmt.Sprintf("%s:%s", target, port),
			success: false,
			message: fmt.Sprintf("❌ Failed to process target %s: %v", target, err),
		}
		return
	}

	// Create a WaitGroup for all the IP checks
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			dialEndpointAsync(ip, port, results)
		}(ip)
	}

	// Wait for all checks to complete
	wg.Wait()
}
