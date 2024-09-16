package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

type Client struct {
	conn *websocket.Conn
	id   string
}

type PopSocket struct {
	clients map[*Client]bool

	serveMux *http.ServeMux
}

type Message struct {
	Event   string `json:"event"`
	Content string `json:"content"`
}

var allowedOrigins []string

func init() {
	allowedOrigins = append(allowedOrigins, "localhost:3000")
}

func (p *PopSocket) broadcastMessage(msg []byte) {
	for client := range p.clients {
		err := client.conn.Write(context.Background(), websocket.MessageText, msg)
		if err != nil {
			log.Printf("Failed to send message to client %s: %v", client.id, err)
			client.conn.CloseNow()
			delete(p.clients, client)
		}
	}
}

func (p *PopSocket) startBroadcasting() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			message := &Message{
				Event:   "connect",
				Content: fmt.Sprintf("Message sent at %s", time.Now().Local().Format(time.RFC1123)),
			}

			msg, err := json.Marshal(message)
			if err != nil {
				log.Printf("Failed to marshal message: %v", err)
				return
			}

			p.broadcastMessage(msg)
		}
	}
}

func main() {
	popsocket := &PopSocket{
		clients:  make(map[*Client]bool),
		serveMux: &http.ServeMux{},
	}
	popsocket.serveMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: allowedOrigins,
		})
		if err != nil {
			log.Println(err)
			return
		}

		client := &Client{
			conn: conn,
			id:   r.Header.Get("Sec-Websocket-Key"),
		}

		popsocket.clients[client] = true
		log.Println("Joined size of connection pool: ", len(popsocket.clients))

		defer func() {
			conn.CloseNow()
			delete(popsocket.clients, client)
			log.Println(client.id, "has disconnected.", "Remaining size of connection pool: ", len(popsocket.clients))
		}()

		for {
			_, _, err := conn.Read(context.Background())
			if err != nil {
				break
			}

			msg, err := json.Marshal(Message{
				Event:   "conversations",
				Content: "",
			})
			if err != nil {
				log.Println(err)
				return
			}

			conn.Write(context.Background(), websocket.MessageText, msg)
		}
	})

	go popsocket.startBroadcasting()

	server := &http.Server{
		Addr:    ":8080",
		Handler: popsocket.serveMux,
	}

	err := server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
