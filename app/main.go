package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"net"
	"strings"
)

// Ensures gofmt doesn't remove the "net" import in stage 1 (feel free to remove this!)
var _ = net.ListenUDP

func main() {
	fmt.Println("Logs from your program will appear here!")
	resolverAddr := flag.String("resolver", "", "resolver address")

	flag.Parse()

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

		fmt.Printf("Received %d bytes from %s\n", size, source)

		if *resolverAddr != "" {
			fmt.Println("resolving using the address")
			forwarder, err := net.Dial("udp", *resolverAddr)
			if err != nil {
				log.Fatal("cannot connect to resolver at:", *resolverAddr, err)
			}
			defer forwarder.Close()

			_, err = forwarder.Write(buf)
			if err != nil {
				fmt.Println("Failed to send to resolver:", err)
			}

			// Read the response
			response := make([]byte, 512)
			n, err := forwarder.Read(response)
			if err != nil {
				log.Fatalf("failed to read response from forwarder: %v", err)
			}

			_, err = udpConn.WriteToUDP(response[:n], source)
			if err != nil {
				fmt.Println("Failed to send response:", err)
			}
			continue
		}

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
		fmt.Println("Qcount", reqQCount)
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
			pointerFlag := (reqQuestion[0] & 0b11000000) >> 6
			if pointerFlag == 0b11 {
				fmt.Println("found pointer")
				pointerAddr := reqQuestion[1]
				fmt.Println("pointer addr", pointerAddr)
				domain, _ := decodeDomain(reqQuestion, buf)
				reqQuestion = reqQuestion[2:]
				domains = append(domains, domain)
				continue
			}

			d, read := decodeDomain(reqQuestion, buf)
			fmt.Println("domain is", d)

			// qType := reqQuestion[:2]
			// qClass := reqQuestion[:2]
			reqQuestion = reqQuestion[read+4:]
			domains = append(domains, d)
		}

		fmt.Println("domain count", len(domains))

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

		binary.BigEndian.PutUint16(header[6:8], qCount)

		// ns count
		header[8] = nsCount[0]
		header[9] = nsCount[1]

		// ar count
		header[10] = addRecordCount[0]
		header[11] = addRecordCount[1]

		fmt.Println("header size", len(header))

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

		}
		response = append(response, question...)

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

			answer = append(answer, []byte{8, 8, 8, 8}...)

		}
		response = append(response, answer...)

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

func decodeDomain(buf []byte, request []byte) (string, int) {
	labels := []string{}
	i := 0
	for {
		pointerFlag := (buf[i] & 0b11000000) >> 6
		if pointerFlag == 0b11 {
			fmt.Println("found pointer")
			i++
			pointerAddr := buf[i]
			i++
			fmt.Println("pointer addr", pointerAddr)
			for request[pointerAddr] != 0 {
				pointerLength := int(request[pointerAddr])
				label := decodeLabel(request[pointerAddr+1:], pointerLength)
				pointerAddr += byte(pointerLength) + 1
				labels = append(labels, label)
			}
			break
		}
		length := int(buf[i])
		// exclude the length bit when parsing
		label := decodeLabel(buf[i+1:], length)
		labels = append(labels, label)
		// length of characters plus first length byte
		i = i + length + 1
		b := buf[i]
		if b == 0 {
			// consume the 0 byte
			i++
			break
		}
	}
	return strings.Join(labels, "."), i
}

func decodeLabel(buf []byte, length int) string {
	label := ""
	i := 0
	for i < length {
		b := buf[i]
		label += string(b)
		i++
	}
	return label
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
