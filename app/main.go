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

		printResponse(buf, size)
		id := binary.BigEndian.Uint16(buf[0:2])

		qr := buf[2] & 128
		qr = qr & 7
		fmt.Println("QR", qr)
		// bits 1 to 4 are opcode
		opcode := buf[2] & 0b01111000
		opcode = opcode >> 3
		fmt.Println("opcode", opcode)

		aa := buf[2] & 0b00000100
		aa = aa >> 2
		fmt.Println("AA", aa)

		tc := buf[2] & 0b00000010
		tc = tc >> 1
		fmt.Println("TC", tc)

		// last bit is rd
		rd := buf[2] & 1
		fmt.Println("RD", rd)

		ra := buf[3] & 128
		ra = ra >> 7
		fmt.Println("RA", ra)

		z := buf[3] & 0b01110000
		z = z >> 4
		fmt.Println("Z", z)

		responseRCode := buf[3] & 0b00001111
		fmt.Println("rcode", responseRCode)

		reqQCount := buf[4:6]
		fmt.Println("qcoutn", reqQCount)
		responseAnCount := buf[6:8]
		fmt.Println("anCount", responseAnCount)

		nsCount := buf[8:10]
		fmt.Println("nsCount", nsCount)

		addRecordCount := buf[10:12]
		fmt.Println("additional record count", addRecordCount)

		// parse question
		reqQuestion := buf[12:]
		qCount := binary.BigEndian.Uint16(reqQCount)
		domains := []string{}
		for i := 0; i < int(qCount); i++ {
			d, read := decodeDomain(reqQuestion)
			reqQuestion = reqQuestion[read:]
			domains = append(domains, d)
		}

		// Create an empty response
		response := []byte{}

		// Header
		header := make([]byte, 12)
		// id
		binary.BigEndian.PutUint16(header[0:2], id)

		// qr opcode aa tc rd
		header[2] = 0b10000000
		mask := opcode << 3
		header[2] |= mask
		header[2] |= rd

		var rcode byte = 0x0
		if opcode != 0 {
			// Not implemented
			rcode = 0x4
		}
		// ra z rcode
		header[3] = rcode

		// question count
		header[4] = reqQCount[0]
		header[5] = reqQCount[1]
		// answer count
		header[6] = 0b00000000
		header[7] = 0b00000001
		response = append(response, header...)

		// Question
		question := []byte{}
		for _, domain := range domains {
			// domain
			question = append(question, encodeDomain(domain)...)
			question = append(question, 0)
			// type
			question = binary.BigEndian.AppendUint16(question, 1)
			// class
			question = binary.BigEndian.AppendUint16(question, 1)

			response = append(response, question...)
		}

		// answer
		answer := []byte{}

		for _, domain := range domains {
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
		}

		_, err = udpConn.WriteToUDP(response, source)
		if err != nil {
			fmt.Println("Failed to send response:", err)
		}
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

func decodeDomain(buf []byte) (string, int) {
	domain := ""
	length := int(buf[0])
	readCount := 0
	i := 1
	for {
		b := buf[i]
		i++
		// If it's terminating 0 then quit
		if b == 0 {
			break
		}
		// If we read the whole label put `.` and continue
		if readCount == length {
			domain += "."
			length = int(b)
			readCount = 0
			continue
		}
		readCount++
		domain += string(b)
	}
	return domain, i
}
func extractBits(b byte, start, end int) byte {
	mask := byte(((1 << (end - start + 1)) - 1) << start)
	return (b & mask) >> start
}

func printResponse(buf []byte, size int) {
	for i := 0; i < size; i++ {
		b := buf[i]
		fmt.Printf("byte #%v: ", i)
		for j := 7; j >= 0; j-- {
			bit := (b >> j) & 1
			fmt.Print(bit)
		}
		fmt.Println()
	}
}
