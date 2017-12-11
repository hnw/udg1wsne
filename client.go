package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	//"time"

	"github.com/jacobsa/go-serial/serial"
	//"github.com/tarm/serial"
)

var (
	semRoutebID = flag.String("u", "", "Route B ID")
	semPassword = flag.String("p", "", "password")
	serialPath  = flag.String("d", "", "device")
)

func main() {
	flag.Parse()

	o := serial.OpenOptions{
		PortName:        *serialPath,
		BaudRate:        115200,
		DataBits:        8,
		StopBits:        1,
		MinimumReadSize: 4,
	}

	// Open the port.
	s, err := serial.Open(o)
	if err != nil {
		log.Fatalf("serial.Open: %v", err)
	}

	reader := bufio.NewReaderSize(s, 4096)

	// Make sure to close it later.
	defer s.Close()

	_, err = s.Write([]byte("\r\n"))
	if err != nil {
		log.Fatal(err)
	}
	_, err = s.Write([]byte("SKRESET\r\n"))
	if err != nil {
		log.Fatal(err)
	}
	_, err = s.Write([]byte("SKVER\r\n"))
	if err != nil {
		log.Fatal(err)
	}
	_, err = s.Write([]byte("SKINFO\r\n"))
	if err != nil {
		log.Fatal(err)
	}

	_, err = s.Write([]byte("SKSETPWD C " + *semPassword + "\r\n"))
	if err != nil {
		log.Fatal(err)
	}

	_, err = s.Write([]byte("SKSETRBID " + *semRoutebID + "\r\n"))
	if err != nil {
		log.Fatal(err)
	}

	_, err = s.Write([]byte("SKSCAN 2 FFFFFFFF 6 0\r\n"))
	if err != nil {
		log.Fatal(err)
	}
	for {
		line, _, err := reader.ReadLine()
		fmt.Println(string(line))
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
	}

}
