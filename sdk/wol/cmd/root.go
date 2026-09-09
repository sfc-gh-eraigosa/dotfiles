package cmd

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	macAddr   string
	bcastAddr string
	port      int
)

var rootCmd = &cobra.Command{
	Use:   "wol [mac_address]",
	Short: "Wake-on-LAN utility",
	Long:  "A simple utility to send magic packets for Wake-on-LAN.",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if macAddr == "" {
			if len(args) > 0 {
				macAddr = args[0]
			} else {
				// Usage text on the way out; a failed write to the help
				// stream has nowhere left to be reported.
				_ = cmd.Help()
				os.Exit(1)
			}
		}

		// Clean up MAC address
		cleanMAC := strings.ReplaceAll(macAddr, ":", "")
		cleanMAC = strings.ReplaceAll(cleanMAC, "-", "")
		cleanMAC = strings.ReplaceAll(cleanMAC, ".", "")

		if len(cleanMAC) != 12 {
			fmt.Printf("Error: Invalid MAC address format: %s\n", macAddr)
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
		addr := fmt.Sprintf("%s:%d", bcastAddr, port)
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
		// Datagram socket: Close reports no delivery information, and the
		// Write below is what actually needs checking.
		defer func() { _ = conn.Close() }()

		_, err = conn.Write(packet)
		if err != nil {
			fmt.Printf("Error sending magic packet: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully sent magic packet for %s to %s\n", macAddr, addr)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&macAddr, "mac", "m", "", "MAC address of the target machine")
	rootCmd.Flags().StringVarP(&bcastAddr, "bcast", "b", "255.255.255.255", "Broadcast address")
	rootCmd.Flags().IntVarP(&port, "port", "p", 9, "UDP port to send magic packet to")
}
