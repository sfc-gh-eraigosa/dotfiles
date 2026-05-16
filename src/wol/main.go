package main

import (
	"bytes"
	"encoding/hex"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
)

func main() {
	macAddr := flag.String("mac", "", "MAC address of the target machine (e.g., b4:2e:99:aa:79:8b)")
	bcastAddr := flag.String("bcast", "255.255.255.255", "Broadcast address (default 255.255.255.255)")
	port := flag.Int("port", 9, "UDP port to send magic packet to (default 9)")
	flag.Parse()

	if *macAddr == "" {
		if flag.NArg() > 0 {
			*macAddr = flag.Arg(0)
		} else {
			fmt.Println("Usage: wol <mac_address> [-bcast <broadcast_ip>] [-port <port>]")
			os.Exit(1)
		}
	}

	// Clean up MAC address
	cleanMAC := strings.ReplaceAll(*macAddr, ":", "")
	cleanMAC = strings.ReplaceAll(cleanMAC, "-", "")
	cleanMAC = strings.ReplaceAll(cleanMAC, ".", "")

	if len(cleanMAC) != 12 {
		fmt.Printf("Error: Invalid MAC address format: %s\n", *macAddr)
		os.Exit(1)
	}

	macBytes, err := hex.DecodeString(cleanMAC)
	if err != nil {
		fmt.Printf("Error decoding MAC address: %v\n", err)
		os.Exit(1)
	}

	// Magic Packet: 6 bytes of 0xFF followed by 16 repetitions of the target MAC
	packet := bytes.Repeat([]byte{0xFF}, 6)
	for i := 0; i < 16; i++ {
		packet = append(packet, macBytes...)
	}

	// Resolve broadcast address
	addr := fmt.Sprintf("%s:%d", *bcastAddr, *port)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		fmt.Printf("Error resolving address: %v\n", err)
		os.Exit(1)
	}

	// Send packet
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		fmt.Printf("Error creating UDP connection: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()

	_, err = conn.Write(packet)
	if err != nil {
		fmt.Printf("Error sending magic packet: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully sent magic packet for %s to %s\n", *macAddr, addr)
}
