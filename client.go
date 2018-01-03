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
		message := []byte{
			// 詳細はECHONET Liteドキュメント「第2部 ECHONET Lite 通信ミドルウェア仕様」を参照のこと
			0x10,       // EHD1 （0x10=ECHONET Lite)
			0x81,       // EHD2 （0x81=電文形式1）
			0x00, 0x01, // TID
			0x05, 0xff, 0x01, // SEOJ （コントローラ）
			0x02, 0x88, 0x01, // DEOJ （低圧スマート電力量メータ）
			0x62, // ESV （プロパティ値読み出し要求）
			0x01, //
			0xe7, // EPC （瞬時電力計測値）
			0x00,
		}
		time.Sleep(500 * time.Millisecond)
		cmd := fmt.Sprintf("SKSENDTO %d %s %04X %d 0 %04X %s", sec, *ipAddr, port, sec, len(message), message)
		udpStatus, _ := SendSkCommand(ch, writer, cmd)
		if strings.HasSuffix(udpStatus, " 00") {
			// UDP送信成功
			ReadEchonetLiteFrame(ch)
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
	m["EDT1"] = frame[28:]   // プロパティ値データ
	return m

	//           SEOJ          ESV   EPC
	//10 81 0001 0EF001 05FF01 72 01 D6 04 01028801
	//10 81 0000 0EF001 0EF001 73 01 D5 04 01028801
	//10 81 0001 028801 05FF01 72 01 E8 04 00140064
	//xxxxxxxxxxxxxxxxxxxxxxxx
}

func ReadEchonetLiteFrame(input chan string) {
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
				m := ParseUdpResponse(line)
				if m["SEOJ"] == "028801" && m["ESV"] == "72" {
					if m["EPC1"] == "E7" {
						v, _ := strconv.ParseInt(m["EDT1"], 16, 0)
						log.Printf("%d [W]\n", v)
						break FOR
					} else if m["EPC1"] == "E8" {
						v1, _ := strconv.ParseInt(m["EDT1"][:4], 16, 0)
						log.Printf("R: %.1f [A]\n", float64(v1)/10.0)
						v2, _ := strconv.ParseInt(m["EDT1"][4:], 16, 0)
						log.Printf("T: %.1f [A]\n", float64(v2)/10.0)
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
