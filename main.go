package main

import "fmt"

func main() {
	var b byte = 0b00000100
	b = b << 1
	printBytes(b)
	fmt.Println(getBit(b, 2))
}

func printBytes(byteValue byte) {
	for i := 7; i >= 0; i-- {
		bit := (byteValue >> i) & 1
		fmt.Print(bit)
	}
	fmt.Println()

}

func getBit(b byte, position uint) byte {
	return (b >> position) & 1
}
