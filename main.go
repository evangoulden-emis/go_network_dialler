package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
)

func main() {
	fileName := "Network Checklist - v1.csv"
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

	for i, record := range records {

		destIp := record[0]
		port := record[1]
		serviceDescription := record[2]
		destinationUrl := record[3]
		urlPort := record[4]
		service := record[5]

		// Skip if IP or port is empty
		if destIp == "" || port == "" {
			fmt.Printf("⚠️ Skipping record %d due to missing IP or port\n", i+1)
			continue
		} else {

		}

		fmt.Println(i, destIp, port, serviceDescription, destinationUrl, urlPort, service)

	}
}

func dialEndpoint(ip string, port string) bool {

}
