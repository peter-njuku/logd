package main

import (
	"net/http"
	"sync"

	"golang.org/x/net/websocket"
)

type client struct {
	ch chan []byte
}

var (
	clientsMu sync.Mutex
	clients   = make(map[*client]struct{})
)

func broadcaster(broadcast <-chan []byte) {
	for msg := range broadcast {
		clientsMu.Lock()
		numClients := len(clients)
		for c := range clients {
			select {
			case c.ch <- msg:
			default:

			}
		}
		clientsMu.Unlock()
		debugf("DEBUG broadcaster: sent to %d clients\n", numClients)
	}
}

func wsHandler() http.Handler {
	return websocket.Handler(func(ws *websocket.Conn) {
		debugf("DEBUG new websocket client connected\n")
		c := &client{
			ch: make(chan []byte, 64),
		}
		clientsMu.Lock()
		clients[c] = struct{}{}
		clientsMu.Unlock()

		defer func() {
			clientsMu.Lock()
			delete(clients, c)
			clientsMu.Unlock()
		}()

		go func() {
			defer ws.Close()

			var discard []byte
			for {
				if _, err := ws.Read(discard); err != nil {
					return
				}
			}
		}()

		for msg := range c.ch {
			debugf("DEBUG wsHandler writing: %s\n", string(msg))
			if err := websocket.Message.Send(ws, msg); err != nil {
				debugf("DEBUG wsHandler send error: %v\n", err)
				return
			}
		}
	})
}
