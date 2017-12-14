package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"

	"github.com/tarm/serial"
)

var (
	semRoutebID = flag.String("u", "", "Route B ID")
	semPassword = flag.String("p", "", "password")
	serialPath  = flag.String("d", "", "device")
)

func main() {
	flag.Parse()

	c := &serial.Config{
		Name:     *serialPath,
		Baud:     115200,
		Size:     8,
		StopBits: 1,
	}

	// Open the port.
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatalf("serial.Open: %v", err)
	}

	//s.Write([]byte("SKVER"))
	//s.WaitResponse([]byte"OK", []byte"NG")

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
