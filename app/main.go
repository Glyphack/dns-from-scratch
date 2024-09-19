package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
)

// Ensures gofmt doesn't remove the "net" import in stage 1 (feel free to remove this!)
var _ = net.ListenUDP

func main() {
	fmt.Println("Logs from your program will appear here!")

	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:2053")
	if err != nil {
		fmt.Println("Failed to resolve UDP address:", err)
		return
	}

	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		fmt.Println("Failed to bind to address:", err)
		return
	}
	defer udpConn.Close()

	buf := make([]byte, 512)

	for {
		size, source, err := udpConn.ReadFromUDP(buf)
		if err != nil {
			fmt.Println("Error receiving data:", err)
			break
		}

		receivedData := string(buf[:size])
		fmt.Printf("Received %d bytes from %s: %s\n", size, source, receivedData)

		// Create an empty response
		response := []byte{}

		// Header
		header := make([]byte, 12)
		// id
		binary.BigEndian.PutUint16(header[0:2], 1234)

		// qr opcode aa tc rd
		header[2] |= 0b10000000

		// ra z rcode
		header[3] = 0b00000000

		// question count
		header[4] = 0b00000000
		header[5] = 0b00000001
		// answer count
		header[6] = 0b00000000
		header[7] = 0b00000001
		response = append(response, header...)

		// Question
		question := []byte{}
		domain := "codecrafters.io"
		question = append(question, encodeDomain(domain)...)
		question = append(question, 0)

		// question - type
		question = binary.BigEndian.AppendUint16(question, 1)
		// question - class
		question = binary.BigEndian.AppendUint16(question, 1)

		fmt.Printf("question: %v\n", question)
		response = append(response, question...)

		// answer
		answer := []byte{}
		answer = append(answer, encodeDomain(domain)...)
		answer = append(answer, 0)

		// 1 for A record and 5 for CNAME
		answer = binary.BigEndian.AppendUint16(answer, 1)

		// class
		answer = binary.BigEndian.AppendUint16(answer, 1)

		// ttl
		answer = binary.BigEndian.AppendUint32(answer, 60)

		// length
		answer = binary.BigEndian.AppendUint16(answer, 4)

		answer = append(answer, []byte("127.0.0.1")...)

		response = append(response, answer...)

		_, err = udpConn.WriteToUDP(response, source)
		if err != nil {
			fmt.Println("Failed to send response:", err)
		}

		fmt.Printf("response: %v\n", response)
	}
}

func encodeDomain(domain string) []byte {
	labels := bytes.Split([]byte(domain), []byte("."))

	encodedValue := []byte{}
	for label := range labels {
		encodedValue = append(encodedValue, byte(len(labels[label])))
		encodedValue = append(encodedValue, labels[label]...)
	}
	return encodedValue
}
