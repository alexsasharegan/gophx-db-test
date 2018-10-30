package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"io/ioutil"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

var (
	isBenchmark = flag.Bool(
		"bench", false, "Runs the benchmark instead of the test",
	)
	verbose = flag.Bool(
		"v", false, "Run with extra logging",
	)
	numClients = flag.Int(
		"procs", runtime.NumCPU()*2,
		"Sets the number of clients that will interact with the DB during the benchmark",
	)
	benchTime = flag.Int(
		"time", 10, "Sets the duration of the benchmark in seconds",
	)
)

func main() {
	if flag.Parse(); *isBenchmark {
		runBenchmark()
	} else {
		runClientTest()
	}
}

func runClientTest() {
	var errCount int
	deadline := 3 * time.Second
	client, err := net.Dial("tcp", ":8888")
	if err != nil {
		log.Fatalf("🚫 Failed dialing client: %v\n", err)
	}
	defer func() {
		client.Write([]byte("QUIT\r\n"))
		client.Close()
	}()
	tx := scanLines(client)

	ttCmds := []struct{ input, output []byte }{
		{[]byte("GET foo\r\n"), []byte("")},
		{[]byte("SET foo bar\r\n"), []byte("OK")},
		{[]byte("GET foo\r\n"), []byte("bar")},
		{[]byte("DEL foo\r\n"), []byte("OK")},
		{[]byte("GET foo\r\n"), []byte("")},
		{[]byte("SET foo a value with spaces\r\n"), []byte("OK")},
		{[]byte("GET foo\r\n"), []byte("a value with spaces")},
	}

	for i, tc := range ttCmds {
		if _, err := client.Write(tc.input); err != nil {
			log.Fatalf(
				"🚫 Failed command test case[%d] while writing input data: %v\n",
				i, err,
			)
		}

		actual := receiveWithDeadline(tx, deadline)
		if !bytes.Equal(actual, tc.output) {
			errCount++
			log.Printf(
				"🚫 Failed command test case[%d]: expected '%s', received '%s'\n",
				i, tc.output, actual,
			)
		}
	}

	ttTrans := []struct{ input, output [][]byte }{
		{
			input: [][]byte{
				[]byte("SET foo baz\r\n"),
				[]byte("GET foo\r\n"),
				[]byte("DEL foo\r\n"),
				[]byte("GET foo\r\n"),
			},
			output: [][]byte{
				[]byte("OK"),
				[]byte("baz"),
				[]byte("OK"),
				[]byte(""),
			},
		},
		{
			input: [][]byte{
				[]byte("SET x 1\r\n"),
				[]byte("GET x\r\n"),
				[]byte("SET x 2\r\n"),
				[]byte("GET x\r\n"),
				[]byte("DEL x\r\n"),
				[]byte("GET x\r\n"),
			},
			output: [][]byte{
				[]byte("OK"),
				[]byte("1"),
				[]byte("OK"),
				[]byte("2"),
				[]byte("OK"),
				[]byte(""),
			},
		},
		{
			input: [][]byte{
				[]byte("DEL foo\r\n"),
				[]byte("GET foo\r\n"),
				[]byte("SET foo 1\r\n"),
				[]byte("GET foo\r\n"),
				[]byte("SET foo 2\r\n"),
				[]byte("GET foo\r\n"),
				[]byte("DEL foo\r\n"),
				[]byte("GET foo\r\n"),
			},
			output: [][]byte{
				[]byte("OK"),
				[]byte(""),
				[]byte("OK"),
				[]byte("1"),
				[]byte("OK"),
				[]byte("2"),
				[]byte("OK"),
				[]byte(""),
			},
		},
	}
	for i, tc := range ttTrans {
		client.Write([]byte("BEGIN\r\n"))
		for j, b := range tc.input {
			if _, err := client.Write(b); err != nil {
				log.Fatalf(
					"🚫 Failed transaction test case[%d], command[%d] while writing input data: %v\n",
					i, j, err,
				)
			}
		}
		client.Write([]byte("COMMIT\r\n"))

		for j, b := range tc.output {
			actual := receiveWithDeadline(tx, deadline)
			if !bytes.Equal(actual, b) {
				errCount++
				log.Printf(
					"🚫 Failed transaction test case[%d], command[%d]: expected '%s', received '%s'\n",
					i, j, string(b), string(actual),
				)
			}
		}
	}

	if errCount == 0 {
		fmt.Println("✅ All tests passed")
	} else {
		fmt.Printf("🚫 Failed %d tests.\n", errCount)
	}
}

func receiveWithDeadline(tx <-chan []byte, d time.Duration) (b []byte) {
	select {
	case <-time.After(d):
		log.Fatalf("🚫 Failed channel receive with deadline %s", d)
	case b = <-tx:
	}

	return b
}

type benchTestClient struct {
	conn         net.Conn
	rx           <-chan []byte
	transactions []benchTransaction
	count        int
}

type benchTransaction struct {
	data  []byte
	count int
}

func runBenchmark() {
	concurrency := *numClients
	benchDuration := time.Second * time.Duration(*benchTime)
	clients := make([]*benchTestClient, concurrency)
	if !*verbose {
		log.SetFlags(0)
		log.SetOutput(ioutil.Discard)
	}

	log.Println(
		fmt.Sprintf("Connecting %d benchmark test clients.", concurrency),
	)
	for i := 0; i < concurrency; i++ {
		btc := new(benchTestClient)
		btc.conn = connectDB()
		defer btc.conn.Close()
		btc.rx = scanLines(btc.conn)
		clients[i] = btc
	}

	log.Println("Generating benchmark test data.")

	var cities = [...]string{
		"phoenix",
		"denver",
		"portland",
		"seattle",
		"omaha",
		"nashville",
		"flagstaff",
		"dallas",
		"austin",
		"houston",
		"tokyo",
		"paris",
		"santiago",
		"bogota",
	}

	for i := 0; i < concurrency; i++ {
		char := fmt.Sprintf("%c", 'a'+i)
		city := cities[i%len(cities)]
		btc := clients[i]
		btc.transactions = []benchTransaction{
			{[]byte(fmt.Sprintf("SET %s client[%d]\r\n", char, i)), 1},
			{[]byte(fmt.Sprintf("GET %s\r\n", char)), 1},
			{[]byte(fmt.Sprintf("DEL %s\r\n", char)), 1},
			{
				bytes.Join(
					[][]byte{
						[]byte("BEGIN\r\n"),
						[]byte(fmt.Sprintf("SET city %s\r\n", city)),
						[]byte("GET city\r\n"),
						[]byte("COMMIT\r\n"),
					},
					nil,
				),
				2,
			},
			{[]byte(fmt.Sprintf("SET client %d\r\n", i)), 1},
			{[]byte("GET client\r\n"), 1},
			{[]byte("DEL client\r\n"), 1},
			{
				bytes.Join(
					[][]byte{
						[]byte("BEGIN\r\n"),
						[]byte("DEL city\r\n"),
						[]byte("GET city\r\n"),
						[]byte("COMMIT\r\n"),
					},
					nil,
				),
				2,
			},
			{[]byte("SET city nowhere\r\n"), 1},
		}
	}

	go func() {
		<-time.After(benchDuration + time.Second)
		log.Fatalln("Benchmark stalled")
	}()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(benchDuration))
	defer cancel()

	for _, btc := range clients {
		go func(btc *benchTestClient) {
			for {
				for _, t := range btc.transactions {
					select {
					case <-ctx.Done():
						return
					default:
						btc.conn.Write(t.data)
						for i := 0; i < t.count; i++ {
							<-btc.rx
						}
						btc.count += t.count
					}
				}
			}
		}(btc)
	}

	log.Println(fmt.Sprintf("Running benchmark for %s", benchDuration))
	<-ctx.Done()
	log.Println("Benchmark complete.")

	var totalMessages int
	for _, btc := range clients {
		totalMessages += btc.count
	}

	p := message.NewPrinter(language.AmericanEnglish)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 4, ' ', tabwriter.AlignRight)
	defer w.Flush()

	results := [][]string{
		{"Clients", "Duration", "Messages Sent & Received"},
		{strconv.Itoa(concurrency), benchDuration.String(), p.Sprintf("%d", totalMessages)},
	}

	for _, r := range results {
		fmt.Fprintln(w, strings.Join(r, "\t")+"\t")
	}
}

func connectDB() net.Conn {
	conn, err := net.Dial("tcp", ":8888")
	if err != nil {
		log.Fatal(err)
	}

	return conn
}

func scanLines(conn net.Conn) <-chan []byte {
	tx := make(chan []byte, 4)

	go func() {
		rd := bufio.NewScanner(conn)
		for rd.Scan() {
			tx <- rd.Bytes()
		}
	}()

	return tx
}

// ScanCRLF is adapted from bufio/scan.go
func ScanCRLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.Index(data, []byte{'\r', '\n'}); i >= 0 {
		// We have a full newline-terminated line.
		return i + 2, data[0:i], nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}
