package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

var verbose bool

func main() {
	udpAddr := flag.String("udp", "", "UDP Address to send our logs to(eg. localhost:514)")
	wsAddr := flag.String("ws", "", "WebSocket address to serve logs on (eg. :8080)")
	verboseFlag := flag.Bool("verbose", false, "Print debug info on stderr")
	help := flag.Bool("help", false, "Show this help message")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <directory>", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options: \n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	verbose = *verboseFlag

	dir := flag.Arg(0)
	if dir == "" {
		log.Fatalf("Usage: %s [options] <directory>", os.Args[0])
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var out io.Writer = os.Stdout
	if *udpAddr != "" {
		conn, err := net.Dial("udp4", *udpAddr)
		if err != nil {
			log.Fatalf("UDP Dial error: %v", err)
		}
		defer conn.Close()
		out = conn
		fmt.Fprintf(os.Stderr, "Sending logs to UDP %s\n", *udpAddr)
	}

	var broadcast chan []byte

	if *wsAddr != "" {
		broadcast = make(chan []byte)
		go broadcaster(broadcast)

		http.Handle("/ws", wsHandler())
		go func() {
			log.Printf("Websocket server listening on %s", *wsAddr)
			if err := http.ListenAndServe(*wsAddr, nil); err != nil {
				log.Fatal(err)
			}
		}()
	}

	var pipeFds [2]int
	if err := unix.Pipe(pipeFds[:]); err != nil {
		log.Fatal(err)
	}
	pipeRead := pipeFds[0]
	pipeWrite := pipeFds[1]
	defer unix.Close(pipeRead)
	defer unix.Close(pipeWrite)

	inotifyFd, err := unix.InotifyInit1(unix.IN_NONBLOCK | unix.IN_CLOEXEC)
	if err != nil {
		log.Fatal(err)
	}
	defer unix.Close(inotifyFd)

	wdDir, err := unix.InotifyAddWatch(inotifyFd, dir, unix.IN_CREATE|unix.IN_DELETE|unix.IN_MOVED_FROM)
	if err != nil {
		log.Fatal(err)
	}

	active := make(map[string]*tailer)
	var wg sync.WaitGroup

	files, err := filepath.Glob(filepath.Join(dir, "*.log"))
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		name := filepath.Base(f)
		startTailer(ctx, filepath.Join(dir, name), name, inotifyFd, active, &wg, out, broadcast)
	}

	go func() {
		<-ctx.Done()
		unix.Write(pipeWrite, []byte{1})
	}()

	buf := make([]byte, 4096)
	for {
		fdSet := &unix.FdSet{}
		fdSet.Bits[inotifyFd/64] |= 1 << (uint(inotifyFd) % 64)
		fdSet.Bits[pipeRead/64] |= 1 << (uint(pipeRead) % 64)

		maxFd := max(pipeRead, inotifyFd)

		_, err := unix.Select(maxFd+1, fdSet, nil, nil, nil)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			log.Fatal(err)
		}
		if (fdSet.Bits[pipeRead/64])&(1<<(uint(pipeRead)%64)) != 0 {
			break
		}

		if (fdSet.Bits[inotifyFd/64])&(1<<(uint(inotifyFd)%64)) != 0 {
			for {
				n, err := unix.Read(inotifyFd, buf)
				if err != nil {
					if err == syscall.EBADF || err == unix.EAGAIN || err == unix.EWOULDBLOCK {
						break
					}
					if err == syscall.EINTR {
						continue
					}
					log.Fatal(err)
				}

				var i uint32
				for i < uint32(n) {
					event := (*unix.InotifyEvent)(unsafe.Pointer(&buf[i]))

					var name string
					if event.Len > 0 {
						nameBytes := buf[i+unix.SizeofInotifyEvent : i+unix.SizeofInotifyEvent+uint32(event.Len)]
						name = strings.TrimRight(string(nameBytes), "\x00")
					}
					if event.Wd == int32(wdDir) {
						if event.Mask&unix.IN_CREATE != 0 {
							debugf("New file detected: %s\n", name)
							if matched, _ := filepath.Match("*.log", name); matched && !strings.HasPrefix(name, ".") {
								if _, exists := active[name]; !exists {
									startTailer(ctx, filepath.Join(dir, name), name, inotifyFd, active, &wg, out, broadcast)
								}
							}
						}
						if event.Mask&unix.IN_DELETE != 0 || event.Mask&unix.IN_MOVED_FROM != 0 {
							if t, ok := active[name]; ok {
								t.cancel()
								t.f.Close()
								delete(active, name)
								debugf("Stopped: %s\n", name)
							}
						}
					} else {
						for basename, t := range active {
							if t.wd == event.Wd {
								if event.Mask&unix.IN_MODIFY != 0 {
									select {
									case t.notify <- struct{}{}:
										debugf("DEBUG notified: %s\n", basename)
									default:
										debugf("DEBUG notify full: %s\n", basename)
									}
								}

								if event.Mask&unix.IN_MOVE_SELF != 0 {
									t.cancel()
									t.f.Close()
									delete(active, basename)
									debugf("Stopped: %s (moved)\n", basename)
								}
								break
							}
						}
					}

					i += unix.SizeofInotifyEvent + uint32(event.Len)
				}
			}
		}
	}

	fmt.Println("\nShuttin down...")
	for _, t := range active {
		t.cancel()
		if t.f != nil {
			t.f.Close()
		}
	}
	debugf("waiting for tailers...")
	wg.Wait()
	fmt.Println("All trailers stopped")
}

func debugf(format string, args ...interface{}) {
	if verbose {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}
