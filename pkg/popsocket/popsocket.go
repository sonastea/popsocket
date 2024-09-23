package popsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

var AllowedOrigins []string

type option func(ps *PopSocket) error

type Client struct {
	id   string
	conn *websocket.Conn
}

type PopSocket struct {
	clients    map[*Client]bool
	httpServer *http.Server
	mu         sync.Mutex
}

type Message struct {
	Event   string `json:"event"`
	Content string `json:"content"`
}

// init initializes default allowed origins for the websocket connections.
func init() {
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins != "" {
		AllowedOrigins = strings.Split(allowedOrigins, ",")
		for i := range AllowedOrigins {
			AllowedOrigins[i] = strings.TrimSpace(AllowedOrigins[i])
		}
	} else {
		AllowedOrigins = []string{"localhost:3000"}
	}
}

// New initializes a new PopSocket with optional configurations.
func New(opts ...option) (*PopSocket, error) {
	ps := &PopSocket{
		clients: make(map[*Client]bool),
		httpServer: &http.Server{
			Addr:    ":80",
			Handler: http.NewServeMux(),
		},
	}

	// Apply any options passed to configure the PopSocket.
	for _, opt := range opts {
		if err := opt(ps); err != nil {
			return nil, err
		}
	}

	return ps, nil
}

// WithServeMux allows setting a custom ServeMux if it's not nil.
func WithServeMux(mux *http.ServeMux) option {
	return func(ps *PopSocket) error {
		if mux != nil {
			ps.httpServer.Handler = mux
		}
		return nil
	}
}

// WithAddress allows setting a custom port for the PopSocket server.
func WithAddress(addr string) option {
	return func(ps *PopSocket) error {
		if addr != "" {
			ps.httpServer.Addr = addr
		}
		return nil
	}
}

// Start launches the PopSocket HTTP server and manages graceful shutdown.
func (p *PopSocket) Start(ctx context.Context) error {
	serverErrors := make(chan error, 1)

	go func() {
		log.Println("Starting PopSocket server on", p.httpServer.Addr)
		if err := p.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	go p.startBroadcasting(ctx)

	select {
	case <-ctx.Done():
		log.Println("Shutting down PopSocket server...")
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return p.httpServer.Shutdown(shutdownCtx)
	case err := <-serverErrors:
		return err
	}
}

// broadcastMessage sends a message to all connected clients.
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

// startBroadcasting periodically sends a message to all connected clients every second.
func (p *PopSocket) startBroadcasting(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
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

// ServeWsHandle handles incoming websocket connection requests.
func (p *PopSocket) ServeWsHandle(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: AllowedOrigins,
	})
	if err != nil {
		log.Println(err)
		return
	}

	client := &Client{
		conn: conn,
		id:   r.Header.Get("Sec-Websocket-Key"),
	}

	p.mu.Lock()
	p.clients[client] = true
	p.mu.Unlock()
	log.Println("Joined size of connection pool: ", len(p.clients))

	defer func() {
		conn.CloseNow()
		delete(p.clients, client)
		log.Println(client.id, "has disconnected.", "Remaining size of connection pool: ", len(p.clients))
	}()

	for {
		_, _, err := conn.Read(r.Context())
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

		conn.Write(r.Context(), websocket.MessageText, msg)
	}
}
