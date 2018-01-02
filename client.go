package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"strconv"
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
		Name:        *serialPath,
		Baud:        115200,
		Size:        8,
		StopBits:    1,
		ReadTimeout: time.Second * 120,
	}

	// Open the port.
	s, err := serial.OpenPort(c)
	if err != nil {
		log.Fatalf("serial.Open: %v", err)
	}

	scanner := bufio.NewScanner(s)
	writer := bufio.NewWriter(s)

	// Make sure to close it later.
	defer s.Close()

	_, _ = SendSkCommand(scanner, writer, "SKVER")

	_, _ = SendSkCommand(scanner, writer, "SKINFO")

	_, _ = SendSkCommand(scanner, writer, "SKSETPWD C "+*semPassword)

	_, _ = SendSkCommand(scanner, writer, "SKSETRBID "+*semRoutebID)

	if *channel == "" || *panId == "" || *ipAddr == "" {
		m := map[string]string{}
		scanDuration := 4
		for scanDuration <= 8 {
			cmd := fmt.Sprintf("SKSCAN 2 FFFFFFFF %d 0", scanDuration)
			_, _ = SendSkCommand(scanner, writer, cmd)
			for scanner.Scan() {
				line := scanner.Text()
				if *debugOutput {
					fmt.Println(line)
				}
				if strings.HasPrefix(line, "EVENT 22 ") {
					break
				}
				if strings.HasPrefix(line, " ") {
					a := strings.SplitN(strings.TrimLeft(line, " "), ":", 2)
					m[a[0]] = a[1]
				}
			}
			if _, ok := m["Channel"]; ok {
				break
			}
			scanDuration++
		}
		if scanDuration > 8 {
			log.Fatal("scan failed")
		}
		*channel = m["Channel"]
		*panId = m["Pan ID"]
		*ipAddr, _ = SendSkCommand(scanner, writer, "SKLL64 "+m["Addr"])
	}

	_, _ = SendSkCommand(scanner, writer, "SKSREG S2 "+*channel)
	_, _ = SendSkCommand(scanner, writer, "SKSREG S3 "+*panId)
	_, _ = SendSkCommand(scanner, writer, "SKJOIN "+*ipAddr)

	for scanner.Scan() {
		line := scanner.Text()
		if *debugOutput {
			fmt.Println(line)
		}
		if strings.HasPrefix(line, "EVENT 24 ") {
			log.Fatal("PANA connection failed")
			break
		}
		if strings.HasPrefix(line, "EVENT 25 ") {
			break
		}
	}

	for scanner.Scan() {
		line := scanner.Text()
		if *debugOutput {
			fmt.Println(line)
		}
		break
	}

	for {
		sec := 1
		port := 3610
		message := []byte{
			0x10, 0x81,
			0x00, 0x01,
			0x05, 0xff, 0x01,
			0x02, 0x88, 0x01,
			0x62,
			0x01,
			0xe7,
			0x00,
		}
		time.Sleep(500 * time.Millisecond)
		cmd := fmt.Sprintf("SKSENDTO %d %s %04X %d 0 %04X %s", sec, *ipAddr, port, sec, len(message), message)
		udpStatus, _ := SendSkCommand(scanner, writer, cmd)
		if udpStatus == "" && scanner.Scan() {
			udpStatus = scanner.Text()
		}
		if strings.HasSuffix(udpStatus, " 00") {
			// UDP送信成功
			for scanner.Scan() {
				line := scanner.Text()
				if *debugOutput {
					fmt.Println(line)
				}
				if strings.HasPrefix(line, "ERXUDP ") {
					m := ParseUdpResponse(line)
					if m["SEOJ"] == "028801" && m["ESV"] == "72" && m["EPC1"] == "E7" {
						v, _ := strconv.ParseInt(m["EDT1"], 16, 0)
						log.Printf("%d [W]\n", v)
						break
					}
				}
			}

		}
	}

	fmt.Println("finished")
}

func SendSkCommand(s *bufio.Scanner, w *bufio.Writer, cmd string) (string, error) {
	res := ""
	if *debugOutput {
		fmt.Println(cmd)
	}
	_, err := w.WriteString(cmd + "\r\n")
	if err != nil {
		log.Fatal(err)
		return "", err
	}
	w.Flush()
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "FAIL ") {
			log.Fatal(line)
		}
		if *debugOutput {
			fmt.Println(line)
		}
		if line == "OK" {
			break
		}
		res += line
		if strings.HasPrefix(cmd, "SKLL64 ") {
			break
		}
	}
	if err := s.Err(); err != nil {
		log.Fatal(err)
	}
	return res, nil
}

func ParseUdpResponse(line string) map[string]string {
	a := strings.Split(line, " ")
	frame := a[9]
	m := map[string]string{}
	m["EHD1"] = frame[0:2]   // ECHONET Lite電文ヘッダー1
	m["EHD2"] = frame[2:4]   // ECHONET Lite電文ヘッダー2
	m["TID"] = frame[4:8]    // トランザクションID
	m["SEOJ"] = frame[8:14]  // 送信元ECHONET Liteオブジェクト指定
	m["DEOJ"] = frame[14:20] // 相手先ECHONET Liteオブジェクト指定
	m["ESV"] = frame[20:22]  // ECHONET Liteサービス
	m["OPC"] = frame[22:24]  // 処理プロパティ数
	m["EPC1"] = frame[24:26] // ECHONET Liteプロパティ
	m["PDC1"] = frame[26:28] // EDTのバイト数
	m["EDT1"] = frame[30:]   // プロパティ値データ
	return m
}
