package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/tarm/serial"
)

var (
	semRoutebID = flag.String("u", "", "Route B ID")
	semPassword = flag.String("p", "", "password")
	serialPath  = flag.String("d", "", "device")
	debugOutput = flag.Bool("debug", false, "debug")
	channel     = flag.String("channel", "", "channel")
	panId       = flag.String("panid", "", "PAN Id")
	ipAddr      = flag.String("ipaddr", "", "IP address")
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
	log.Printf("Opening serial port: %s", *serialPath)
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatalf("serial.Open: %v", err)
	}

	scanner := bufio.NewScanner(s)
	writer := bufio.NewWriter(s)

	ch := make(chan string, 4)

	go func() {
		defer close(ch)
		defer s.Close()

		for scanner.Scan() {
			line := scanner.Text()
			ch <- line
		}
		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
	}()

	// Make sure to close it later.

	_, _ = SendSkCommand(ch, writer, "SKVER")

	_, _ = SendSkCommand(ch, writer, "SKINFO")

	_, _ = SendSkCommand(ch, writer, "SKSETPWD C "+*semPassword)

	_, _ = SendSkCommand(ch, writer, "SKSETRBID "+*semRoutebID)

	if *channel == "" || *panId == "" || *ipAddr == "" {
		scanDuration := 4
		for scanDuration <= 8 {
			cmd := fmt.Sprintf("SKSCAN 2 FFFFFFFF %d 0", scanDuration)
			_, _ = SendSkCommand(ch, writer, cmd)
			m := ReadScan(ch)
			if _, ok := m["Channel"]; ok {
				*channel = m["Channel"]
				*panId = m["Pan ID"]
				*ipAddr, _ = SendSkCommand(ch, writer, "SKLL64 "+m["Addr"])
				break
			}
			scanDuration++
		}
		if scanDuration > 8 {
			log.Fatal("scan failed")
		}
	}

	_, _ = SendSkCommand(ch, writer, "SKSREG S2 "+*channel)
	_, _ = SendSkCommand(ch, writer, "SKSREG S3 "+*panId)

	log.Print("Starting PANA")
	_, _ = SendSkCommand(ch, writer, "SKJOIN "+*ipAddr)
	ReadPana(ch)
	log.Print("PANA Completed")

	for {
		sec := 1
		port := 3610
		req := NewEchoFrame(SmartElectricMeter, Get, InstantaneousElectricPower, []byte{})

		time.Sleep(500 * time.Millisecond)
		rawFrame := req.Build()
		cmd := fmt.Sprintf("SKSENDTO %d %s %04X %d 0 %04X %s", sec, *ipAddr, port, sec, len(rawFrame), rawFrame)
		udpStatus, _ := SendSkCommand(ch, writer, cmd)
		if strings.HasSuffix(udpStatus, " 00") {
			// UDP送信成功
			ReadEchonetLiteFrame(ch, req)
		}
	}
	fmt.Println("finished")
}

func SendSkCommand(input chan string, w *bufio.Writer, cmd string) (string, error) {
	_, err := w.WriteString(cmd + "\r\n")
	if err != nil {
		log.Fatal(err)
		return "", err
	}
	w.Flush()
	if *debugOutput {
		fmt.Println(cmd)
	}

	res := ""
	timeout := 60 * time.Second
	tm := time.NewTimer(timeout)
FOR:
	for {
		select {
		case <-tm.C:
			log.Println("read timeout 9")
			break FOR
		case line, ok := <-input:
			if !ok {
				break FOR
			}
			if strings.HasPrefix(line, "FAIL ") {
				log.Fatal(line)
			}
			if *debugOutput {
				fmt.Println(line)
			}
			if line == "OK" {
				break FOR
			}
			res += line
			if strings.HasPrefix(cmd, "SKLL64 ") {
				// SKLL64コマンドだけはOKを返さない
				break FOR
			}
			tm.Reset(timeout)
		}
	}
	return res, nil
}

func ParseUdpResponse(line string) *echoFrame {
	a := strings.Split(line, " ")
	decoded, _ := hex.DecodeString(a[9])
	fr, _ := ParseEchoFrame(decoded)
	return fr
}

func ReadEchonetLiteFrame(input chan string, req *echoFrame) {
	timeout := 2 * time.Second
	tm := time.NewTimer(timeout)
FOR:
	for {
		select {
		case <-tm.C:
			log.Println("read timeout")
			break FOR
		case line, ok := <-input:
			if !ok {
				break FOR
			}
			if *debugOutput {
				fmt.Println(line)
			}
			if strings.HasPrefix(line, "ERXUDP ") {
				res := ParseUdpResponse(line)
				if req.CorrespondTo(res) {
					if req.EPC[0] == InstantaneousElectricPower {
						v := binary.BigEndian.Uint32(res.EDT[0])
						log.Printf("%d [W]\n", v)
						break FOR
					} else if req.EPC[0] == InstantaneousCurrents {
						r := binary.BigEndian.Uint16(res.EDT[0][0:2])
						t := binary.BigEndian.Uint16(res.EDT[0][2:4])
						log.Printf("R-phase: %.1f [A]\n", float64(r)/10.0)
						log.Printf("T-phase: %.1f [A]\n", float64(t)/10.0)
						break FOR
					}
				}
			}
			tm.Reset(timeout)
		}
	}
}

func ReadPana(input chan string) {
	timeout := 10 * time.Second
	tm := time.NewTimer(timeout)
FOR:
	for {
		select {
		case <-tm.C:
			log.Println("read timeout")
			break FOR
		case line, ok := <-input:
			if !ok {
				break FOR
			}
			if *debugOutput {
				fmt.Println(line)
			}
			if strings.HasPrefix(line, "EVENT 24 ") {
				log.Fatal("PANA connection failed")
			}
			if strings.HasPrefix(line, "EVENT 25 ") {
				break FOR
			}
			tm.Reset(timeout)
		}
	}
}

func ReadScan(input chan string) map[string]string {
	m := map[string]string{}

	timeout := 120 * time.Second
	tm := time.NewTimer(timeout)
FOR:
	for {
		select {
		case <-tm.C:
			log.Println("read timeout")
			break FOR
		case line, ok := <-input:
			if !ok {
				break FOR
			}
			if *debugOutput {
				fmt.Println(line)
			}
			if strings.HasPrefix(line, "EVENT 22 ") {
				break FOR
			}
			if strings.HasPrefix(line, " ") {
				a := strings.SplitN(strings.TrimLeft(line, " "), ":", 2)
				m[a[0]] = a[1]
			}

			tm.Reset(timeout)
		}
	}
	return m
}
