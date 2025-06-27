package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
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

	for i, record := range records[1:] { // Skip header row

		destIp := record[0]
		port := record[1]

		// Skip if IP or port is empty
		if destIp == "" {
			fmt.Printf("⚠️ Skipping record %d due to missing IP", i+1)
			continue
		} else if parseIpAddress(destIp) == false {
			fmt.Printf("⚠️ Skipping record %d due to invalid IP: %s\n", i+1, destIp)
			continue
		}
		if port == "" {
			port = "443"
		}
		wg.Add(1)
		go func(ip, port string) {
			defer wg.Done()
			dialEndpointAsync(ip, port, results)
		}(destIp, port)

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

func parseIpAddress(ip string) bool {
	_, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return true
}

func dialEndpoint(ip string, port string) {
	address := net.JoinHostPort(ip, port)
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)

	if err != nil {
		switch {
		case errors.Is(err, os.ErrDeadlineExceeded):
			fmt.Printf("⏳ Connection timeout to %s\n", address)
		case errors.Is(err, syscall.ECONNREFUSED):
			fmt.Printf("🛑 Connection refused to %s\n", address)
		case errors.Is(err, syscall.ENETUNREACH):
			fmt.Printf("🤷🏼‍♂️ Network unreachable for %s\n", address)
		default:
			// Handle other types of errors
			fmt.Printf("Failed to connect to %s: %v\n", address, err)

		}

	} else {
		fmt.Printf("✅ Connected to %s\n", address)
		defer func(conn net.Conn) {
			err := conn.Close()
			if err != nil {
				fmt.Println("Failed to close the connection cleanly: ", err)
			}
		}(conn)
	}

}

func dialEndpointAsync(ip string, port string, results chan<- CheckResult) {
	address := net.JoinHostPort(ip, port)

	conn, err := net.DialTimeout("tcp", address, time.Second*3)
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
