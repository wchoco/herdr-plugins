//go:build ignore

// bench measures round trips from WSL to winhelper.exe. Run from WSL:
//
//	go run bench.go -exe /path/to/winhelper.exe -mode spawn -n 200
//
// Modes: spawn (one process per call), pipe (one long-lived process,
// line echo over stdin/stdout), tcp (one connection, line echo),
// dialeach (new TCP connection per call), accept (WSL listens on
// 127.0.0.1, winhelper.exe dials in through localhost forwarding and
// echoes), serve (plain echo server for winhelper.exe dial).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sort"
	"time"
)

func report(name string, d []time.Duration) {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	p := func(q float64) time.Duration { return d[int(q*float64(len(d)-1))] }
	ms := func(x time.Duration) string { return fmt.Sprintf("%.2f", float64(x)/1e6) }
	fmt.Printf("%-28s n=%d min=%s p50=%s p95=%s p99=%s max=%s ms\n",
		name, len(d), ms(d[0]), ms(p(.5)), ms(p(.95)), ms(p(.99)), ms(d[len(d)-1]))
}

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func main() {
	exe := flag.String("exe", "", "program to run")
	mode := flag.String("mode", "spawn", "spawn | pipe | tcp | dialeach")
	n := flag.Int("n", 200, "iterations")
	addr := flag.String("addr", "127.0.0.1:47651", "tcp address")
	name := flag.String("name", "", "label")
	gap := flag.Duration("gap", 5*time.Millisecond, "pause between round trips")
	flag.Parse()
	label := *name
	if label == "" {
		label = *mode
	}
	var d []time.Duration
	switch *mode {
	case "spawn":
		args := flag.Args()
		for i := 0; i < *n; i++ {
			t := time.Now()
			if err := exec.Command(*exe, args...).Run(); err != nil {
				die(err)
			}
			d = append(d, time.Since(t))
		}
	case "pipe":
		t0 := time.Now()
		cmd := exec.Command(*exe, "echo")
		in, _ := cmd.StdinPipe()
		out, _ := cmd.StdoutPipe()
		if err := cmd.Start(); err != nil {
			die(err)
		}
		r := bufio.NewReader(out)
		fmt.Fprintln(in, "warm")
		if _, err := r.ReadString('\n'); err != nil {
			die(err)
		}
		fmt.Printf("pipe start+first round trip: %.2f ms\n", float64(time.Since(t0))/1e6)
		for i := 0; i < *n; i++ {
			t := time.Now()
			fmt.Fprintln(in, "off")
			if _, err := r.ReadString('\n'); err != nil {
				die(err)
			}
			d = append(d, time.Since(t))
			time.Sleep(*gap)
		}
		in.Close()
		cmd.Wait()
	case "tcp":
		t0 := time.Now()
		c, err := net.DialTimeout("tcp", *addr, 2*time.Second)
		if err != nil {
			fmt.Println("tcp dial failed:", err)
			os.Exit(3)
		}
		fmt.Printf("tcp connect: %.2f ms\n", float64(time.Since(t0))/1e6)
		r := bufio.NewReader(c)
		for i := 0; i < *n; i++ {
			t := time.Now()
			fmt.Fprintln(c, "off")
			if _, err := r.ReadString('\n'); err != nil {
				die(err)
			}
			d = append(d, time.Since(t))
			time.Sleep(*gap)
		}
		c.Close()
	case "dialeach":
		for i := 0; i < *n; i++ {
			t := time.Now()
			c, err := net.DialTimeout("tcp", *addr, 2*time.Second)
			if err != nil {
				fmt.Println("tcp dial failed:", err)
				os.Exit(3)
			}
			fmt.Fprintln(c, "off")
			if _, err := bufio.NewReader(c).ReadString('\n'); err != nil {
				die(err)
			}
			c.Close()
			d = append(d, time.Since(t))
		}
	case "accept":
		// Listen in WSL, start `winhelper.exe dialecho` (via -exe), and
		// time round trips over the connection it makes.
		ln, err := net.Listen("tcp", *addr)
		if err != nil {
			die(err)
		}
		t0 := time.Now()
		cmd := exec.Command(*exe, "dialecho", *addr)
		if err := cmd.Start(); err != nil {
			die(err)
		}
		c, err := ln.Accept()
		if err != nil {
			die(err)
		}
		ln.Close()
		fmt.Printf("spawn+connect: %.2f ms\n", float64(time.Since(t0))/1e6)
		r := bufio.NewReader(c)
		for i := 0; i < *n; i++ {
			t := time.Now()
			fmt.Fprintln(c, "off")
			if _, err := r.ReadString('\n'); err != nil {
				die(err)
			}
			d = append(d, time.Since(t))
			time.Sleep(*gap)
		}
		c.Close()
		cmd.Wait()
	case "serve":
		// Echo server in WSL on 127.0.0.1 for `winhelper.exe dial`;
		// it stops after the first connection closes.
		ln, err := net.Listen("tcp", *addr)
		if err != nil {
			die(err)
		}
		fmt.Println("listening")
		c, err := ln.Accept()
		if err != nil {
			die(err)
		}
		r := bufio.NewScanner(c)
		for r.Scan() {
			c.Write(append(r.Bytes(), '\n'))
		}
		c.Close()
		ln.Close()
		return
	default:
		die(fmt.Errorf("unknown mode %q", *mode))
	}
	report(label, d)
}
