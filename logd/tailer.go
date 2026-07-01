package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

type tailer struct {
	cancel context.CancelFunc
	notify chan struct{}
	wd     int32
	f      *os.File
}

func tailFile(ctx context.Context, f *os.File, prefix string, out io.Writer, notify <-chan struct{}) {
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
			fmt.Fprintf(out, "[%s] - %s", prefix, line)
		}
		select {
		case <-ctx.Done():
			return
		case <-notify:
		}
	}
}

func startTailer(ctx context.Context, path, prefix string, inotifyFd int, active map[string]*tailer, wg *sync.WaitGroup) {
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
		tailFile(childCtx, f, prefix, os.Stdout, t.notify)
	}()
}
