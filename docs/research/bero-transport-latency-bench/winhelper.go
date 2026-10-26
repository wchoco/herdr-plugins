//go:build ignore

// winhelper is a no-op stand-in for an IME Agent, built with GOOS=windows.
// It never touches the IME. Modes:
//
//	winhelper.exe          exit immediately (per-call spawn cost)
//	winhelper.exe echo     echo stdin lines to stdout until EOF
//	winhelper.exe tcp ADDR echo lines on a TCP listener bound to ADDR
//	                       (127.0.0.1 only); exits after 60 s
//	winhelper.exe dial ADDR N
//	                       time N line round trips to an echo server
//	                       (Windows clock is coarse; prefer dialecho)
//	winhelper.exe dialecho ADDR
//	                       connect to ADDR and echo lines back
package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func echo(r io.Reader, w io.Writer) {
	sc := bufio.NewScanner(r)
	bw := bufio.NewWriter(w)
	for sc.Scan() {
		bw.Write(sc.Bytes())
		bw.WriteByte('\n')
		bw.Flush()
	}
}

func dial(addr, count string) {
	n, _ := strconv.Atoi(count)
	t0 := time.Now()
	c, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	fmt.Printf("connect %d\n", time.Since(t0).Microseconds())
	r := bufio.NewReader(c)
	for i := 0; i < n; i++ {
		t := time.Now()
		c.Write([]byte("off\n"))
		if _, err := r.ReadString('\n'); err != nil {
			os.Exit(1)
		}
		fmt.Println(time.Since(t).Microseconds())
		time.Sleep(5 * time.Millisecond)
	}
}

func main() {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[1] {
	case "echo":
		echo(os.Stdin, os.Stdout)
	case "dial":
		// dial ADDR N: connect to an echo server (in WSL) and time N
		// line round trips, printing each in microseconds.
		dial(os.Args[2], os.Args[3])
	case "dialecho":
		// dialecho ADDR: connect to bench's "accept" mode in WSL and
		// echo what it sends; the WSL side does the timing.
		c, err := net.DialTimeout("tcp", os.Args[2], 2*time.Second)
		if err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		echo(c, c)
	case "tcp":
		addr := os.Args[2]
		if !strings.HasPrefix(addr, "127.0.0.1:") {
			os.Exit(2)
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
			os.Exit(1)
		}
		os.Stdout.WriteString("listening\n")
		go func() { time.Sleep(60 * time.Second); os.Exit(0) }()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); echo(c, c) }()
		}
	}
}
