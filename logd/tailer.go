package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type tailer struct {
	cancel context.CancelFunc
	notify chan struct{}
	wd     int32
	f      *os.File
}

type logEntry struct {
	File      string `json:"file"`
	Line      string `json:"line"`
	Timestamp string `json:"timestamp"`
}

func tailFile(ctx context.Context, f *os.File, prefix string, out io.Writer, notify <-chan struct{}, broadcast chan<- []byte) {
	reader := bufio.NewReader(f)
	for {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				if errors.Is(err, io.EOF) {
					break
				}
				fmt.Fprintf(os.Stderr, "Read error on %s: %v", prefix, err)
				return
			}

			cleanLine := strings.TrimRight(line, "\r\n")

			entry := logEntry{
				File:      prefix,
				Line:      cleanLine,
				Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			}

			jsonBytes, err := json.Marshal(entry)
			if err != nil {
				fmt.Fprintf(os.Stderr, "JSON Marshal Error on %s: %v", prefix, err)
				continue
			}
			//fmt.Fprintf(out, "%s\n", string(jsonBytes))
			debugf("DEBUG writing %d bytes for %s: %s\n", len(jsonBytes), prefix, string(jsonBytes))
			out.Write(jsonBytes)
			if broadcast != nil {
				msg := make([]byte, len(jsonBytes))
				copy(msg, jsonBytes)
				select {
				case broadcast <- msg:
					fmt.Fprintf(os.Stderr, "DEBUG broadcast sent\n")
				default:
					fmt.Fprintf(os.Stderr, "Broadcast channel full, dropping message for %s\n", prefix)
				}
			}
			out.Write([]byte{'\n'})
		}
		select {
		case <-ctx.Done():
			return
		case <-notify:
		}
	}
}

func startTailer(ctx context.Context, path, prefix string, inotifyFd int, active map[string]*tailer, wg *sync.WaitGroup, out io.Writer, broadcast chan<- []byte) {
	f, err := os.Open(path)
	if err != nil {
		log.Printf("Cannot open file %s: %v", path, err)
		return
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		log.Printf("Seek error: %v", err)
		f.Close()
		return
	}

	childCtx, cancel := context.WithCancel(ctx)
	t := &tailer{
		cancel: cancel,
		notify: make(chan struct{}, 1),
		f:      f,
	}

	wd, err := unix.InotifyAddWatch(inotifyFd, path, unix.IN_MODIFY|unix.IN_MOVE_SELF)
	if err != nil {
		log.Printf("Watch error: %v", err)
		f.Close()
		return
	}

	t.wd = int32(wd)
	active[prefix] = t
	wg.Add(1)
	go func() {
		defer wg.Done()
		tailFile(childCtx, f, prefix, out, t.notify, broadcast)
	}()
}
