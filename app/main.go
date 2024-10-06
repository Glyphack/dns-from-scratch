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

type DnsMessage struct {
	Header    Header
	Questions []Question
	Answers   []Answer
	Size      int
}

type Answer struct {
	Name   []string
	Type   uint16
	Class  uint16
	TTL    uint32
	Length uint16
	RData  []byte
}

type Question struct {
	// domain name as labels
	Name  []string
	Type  uint16
	Class uint16
}

type Header struct {
	Id                  uint16
	QueryResponse       bool
	Opcode              byte
	AuthoritativeAnswer bool
	TruncatedMessage    bool
	RecursionDesired    bool
	RecursionAvailable  bool
	Z                   byte
	Reserved            byte
	ResponseCode        byte
	QuestionCount       uint16
	AnswerCount         uint16
	AuthorityCount      uint16
	AdditionalCount     uint16
}

func ParseDnsMessage(buf []byte, size int) DnsMessage {
	id := binary.BigEndian.Uint16(buf[0:2])

	qrT := buf[2] & 0b10000000
	qr := qrT == 0b10000000

	opcode := buf[2] & 0b01111000
	opcode = opcode >> 3

	aaT := buf[2] & 0b00000100
	aa := aaT == 0b00000100

	tcT := buf[2] & 0b00000010
	tc := tcT == 0b00000010

	// last bit is rd
	rdT := buf[2] & 1
	rd := rdT == 1

	raT := buf[3] & 128
	ra := raT == 1

	z := buf[3] & 0b01110000
	z = z >> 4

	// rcode is set by server so ignore
	_ = buf[3] & 0b00001111

	reqQCount := binary.BigEndian.Uint16(buf[4:6])
	answerCount := binary.BigEndian.Uint16(buf[6:8])
	nsCount := binary.BigEndian.Uint16(buf[8:10])
	additionalCount := binary.BigEndian.Uint16(buf[10:12])

	reqQuestion := buf[12:]
	qCount := reqQCount
	questions := make([]Question, qCount)
	for i := 0; i < int(qCount); i++ {
		var domain []string
		pointerFlag := (reqQuestion[0] & 0b11000000) >> 6
		if pointerFlag == 0b11 {
			pointerAddr := reqQuestion[1]
			domain, _ = decodeDomain(buf[pointerAddr:], buf)
			reqQuestion = reqQuestion[2:]
		} else {
			var readLen int
			domain, readLen = decodeDomain(reqQuestion, buf)
			reqQuestion = reqQuestion[readLen:]
		}
		_ = binary.BigEndian.Uint16(reqQuestion[:2])
		reqQuestion = reqQuestion[2:]
		_ = binary.BigEndian.Uint16(reqQuestion[:2])
		reqQuestion = reqQuestion[2:]

		questions[i] = Question{
			Name:  domain,
			Type:  1,
			Class: 1,
		}
	}

	answers := make([]Answer, int(answerCount))
	for i := 0; i < int(answerCount); i++ {
		var domain []string
		pointerFlag := (reqQuestion[0] & 0b11000000) >> 6
		if pointerFlag == 0b11 {
			pointerAddr := reqQuestion[1]
			domain, _ = decodeDomain(buf[pointerAddr:], buf)
			reqQuestion = reqQuestion[2:]
		} else {
			var readLen int
			domain, readLen = decodeDomain(reqQuestion, buf)
			reqQuestion = reqQuestion[readLen:]
		}

		qType := binary.BigEndian.Uint16(reqQuestion[:2])
		reqQuestion = reqQuestion[2:]
		qClass := binary.BigEndian.Uint16(reqQuestion[:2])
		reqQuestion = reqQuestion[2:]
		ttl := binary.BigEndian.Uint32(reqQuestion[:4])
		reqQuestion = reqQuestion[4:]
		length := binary.BigEndian.Uint16(reqQuestion[:2])
		reqQuestion = reqQuestion[2:]
		rData := reqQuestion[:4]

		answers[i] = Answer{
			Name:   domain,
			Type:   qType,
			Class:  qClass,
			TTL:    ttl,
			Length: length,
			RData:  rData,
		}
	}

	return DnsMessage{
		Header: Header{
			Id:                  id,
			QueryResponse:       qr,
			Opcode:              opcode,
			AuthoritativeAnswer: aa,
			TruncatedMessage:    tc,
			RecursionDesired:    rd,
			RecursionAvailable:  ra,
			Z:                   z,
			Reserved:            0,
			ResponseCode:        0,
			QuestionCount:       qCount,
			AnswerCount:         answerCount,
			AuthorityCount:      nsCount,
			AdditionalCount:     additionalCount,
		},
		Questions: questions,
		Answers:   answers,
		Size:      size,
	}
}

func (reqDnsMessage DnsMessage) EncodeDnsMessage() []byte {
	encoded := make([]byte, 512)

	// Header
	header := make([]byte, 12)
	// id
	binary.BigEndian.PutUint16(header[0:2], reqDnsMessage.Header.Id)

	header[2] |= boolToUint8(reqDnsMessage.Header.QueryResponse) << 7

	// opcode
	mask := reqDnsMessage.Header.Opcode << 3
	header[2] |= mask
	// aa
	header[2] &^= boolToUint8(reqDnsMessage.Header.AuthoritativeAnswer) << 2
	// tc
	header[2] &^= boolToUint8(reqDnsMessage.Header.TruncatedMessage) << 1
	// rd
	header[2] |= boolToUint8(reqDnsMessage.Header.RecursionDesired)

	// ra
	header[3] |= boolToUint8(reqDnsMessage.Header.RecursionAvailable) << 7
	// z
	header[3] |= reqDnsMessage.Header.Z << 4
	// rcode
	header[3] |= reqDnsMessage.Header.ResponseCode

	// question count
	binary.BigEndian.PutUint16(header[4:6], uint16(len(reqDnsMessage.Questions)))

	// answer count assume it's same as question count
	binary.BigEndian.PutUint16(header[6:8], uint16(len(reqDnsMessage.Answers)))

	// just pass the value from request
	binary.BigEndian.PutUint16(header[8:10], reqDnsMessage.Header.AuthorityCount)
	binary.BigEndian.PutUint16(header[10:12], reqDnsMessage.Header.AdditionalCount)

	i := 0
	for i < len(header) {
		encoded[i] = header[i]
		i++
	}

	rest := []byte{}
	// Question
	questionSection := []byte{}
	for _, question := range reqDnsMessage.Questions {
		// domain
		d := strings.Join(question.Name, ".")
		questionSection = append(questionSection, encodeDomain(d)...)
		questionSection = append(questionSection, 0)
		// type
		questionSection = binary.BigEndian.AppendUint16(questionSection, 1)
		// class
		questionSection = binary.BigEndian.AppendUint16(questionSection, 1)

	}
	rest = append(rest, questionSection...)

	// answerSection
	answerSection := []byte{}
	for _, answer := range reqDnsMessage.Answers {
		d := strings.Join(answer.Name, ".")
		answerSection = append(answerSection, encodeDomain(d)...)
		answerSection = append(answerSection, 0)
		answerSection = binary.BigEndian.AppendUint16(answerSection, answer.Type)
		answerSection = binary.BigEndian.AppendUint16(answerSection, answer.Class)
		answerSection = binary.BigEndian.AppendUint32(answerSection, answer.TTL)
		answerSection = binary.BigEndian.AppendUint16(answerSection, answer.Length)
		answerSection = append(answerSection, answer.RData...)
	}
	rest = append(rest, answerSection...)

	i = 0
	for i < len(rest) {
		encoded[i+12] = rest[i]
		i++
	}

	return encoded
}

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

		incomingDnsMessage := ParseDnsMessage(buf, size)

		respDnsMessage := incomingDnsMessage
		// Resolver is used to resolve messages one by one
		if *resolverAddr != "" {
			fmt.Println("resolving using the address", *resolverAddr)
			forwarder, err := net.Dial("udp", *resolverAddr)
			if err != nil {
				log.Fatal("cannot connect to resolver at:", *resolverAddr, err)
			}
			defer forwarder.Close()
			// TODO: enable after implementing the compression
			// if !slices.Equal(myInput, buf) {
			// 	log.Fatalf("Not equal:\n %v \n %#v \n %v\n", buf, incomingDnsMessage, myInput)
			// }

			answers := []Answer{}
			for _, q := range incomingDnsMessage.Questions {
				resolverReq := incomingDnsMessage
				resolverReq.Header.QuestionCount = 1
				resolverReq.Questions = []Question{q}

				_, err = forwarder.Write(resolverReq.EncodeDnsMessage())
				if err != nil {
					fmt.Println("Failed to send to resolver:", err)
				}

				ResolverResp := make([]byte, 1024)
				n, err := forwarder.Read(ResolverResp)
				if err != nil {
					log.Fatalf("failed to read response from forwarder: %v", err)
				}
				resolverResponseDnsMsg := ParseDnsMessage(ResolverResp, n)
				fmt.Printf("Resolver response: %+v\n", resolverResponseDnsMsg)
				answers = append(answers, resolverResponseDnsMsg.Answers...)
			}
			respDnsMessage.Answers = answers
		} else {
			for _, domain := range incomingDnsMessage.Questions {
				answer := Answer{}
				answer.Name = domain.Name
				answer.Class = 1
				answer.Type = 1
				answer.TTL = 60
				answer.Length = 4
				answer.RData = []byte{8, 8, 8, 8}
				respDnsMessage.Answers = append(respDnsMessage.Answers, answer)
			}
		}

		respDnsMessage.Header.AnswerCount = uint16(len(respDnsMessage.Answers))
		var rcode byte = 0
		if incomingDnsMessage.Header.Opcode != 0 {
			// Not implemented
			rcode = 0b00000100
		}

		respDnsMessage.Header.Id = incomingDnsMessage.Header.Id
		respDnsMessage.Header.QueryResponse = true
		respDnsMessage.Header.AuthoritativeAnswer = false
		respDnsMessage.Header.TruncatedMessage = false
		respDnsMessage.Header.ResponseCode = rcode
		if incomingDnsMessage.Header.RecursionDesired {
			respDnsMessage.Header.RecursionAvailable = true
		}

		fmt.Printf("Own response: %+v\n", respDnsMessage)
		ownResp := respDnsMessage.EncodeDnsMessage()

		_, err = udpConn.WriteToUDP(ownResp, source)
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

func decodeDomain(buf []byte, request []byte) ([]string, int) {
	labels := []string{}
	i := 0
	for {
		pointerFlag := (buf[i] & 0b11000000) >> 6
		if pointerFlag == 0b11 {
			i++
			pointerAddr := buf[i]
			i++
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
	return labels, i
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
func boolToUint8(val bool) uint8 {
	if val {
		return 1
	} else {
		return 0
	}
}
