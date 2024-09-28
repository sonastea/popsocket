package popsocket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

var (
	AllowedOrigins []string

	_ PopSocketInterface = (*PopSocket)(nil)
)

type option func(ps *PopSocket) error

type Client struct {
	id   string
	conn *websocket.Conn
}

type PopSocketInterface interface {
	LogDebug(string, ...any)
	LogError(string, ...any)
	LogInfo(string, ...any)
	LogWarn(string, ...any)

	Start(ctx context.Context) error
	ServeWsHandle(w http.ResponseWriter, r *http.Request)
}

type PopSocket struct {
	clients    map[*Client]bool
	httpServer *http.Server
	logger     *slog.Logger
	mu         sync.Mutex
}

type Message struct {
	Event   string `json:"event"`
	Content string `json:"content"`
}

// init initializes default allowed origins for the websocket connections.
func init() {
	loadAllowedOrigins()
}

func loadAllowedOrigins() {
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
		logger: newLogger(slog.NewJSONHandler(os.Stdout, nil)),
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
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGINT, syscall.SIGKILL)
	defer cancel()

	serverErrors := make(chan error, 1)

	go func() {
		p.logger.Info(fmt.Sprintf("Starting PopSocket server on %v", p.httpServer.Addr))
		if err := p.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	go p.startBroadcasting(ctx)

	select {
	case <-ctx.Done():
		p.logger.Info(fmt.Sprintf("Shutting down PopSocket server..."))
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return p.httpServer.Shutdown(shutdownCtx)
	case err := <-serverErrors:
		return err
	}
}

// LogDebug logs a debug message to the underlying slog logger.
func (p *PopSocket) LogDebug(msg string, args ...any) {
	p.logger.Debug(msg, args...)
}

// LogError logs an error message to the underlying slog logger.
func (p *PopSocket) LogError(msg string, args ...any) {
	p.logger.Error(msg, args...)
}

// LogInfo logs an info message to the underlying slog logger.
func (p *PopSocket) LogInfo(msg string, args ...any) {
	p.logger.Info(msg, args...)
}

// LogWarn logs a warning message to the underlying slog logger.
func (p *PopSocket) LogWarn(msg string, args ...any) {
	p.logger.Warn(msg, args...)
}

// addClient safely adds a client to the clients map.
func (p *PopSocket) addClient(client *Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.clients[client] = true
}

// removeClient safely removes a client from the clients map.
func (p *PopSocket) removeClient(client *Client) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.clients, client)
}

// getClientCount returns the number of connected clients.
func (p *PopSocket) totalClients() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.clients)
}

// broadcastMessage sends a message to all connected clients.
func (p *PopSocket) broadcastMessage(msg []byte) {
	for client := range p.clients {
		err := client.conn.Write(context.Background(), websocket.MessageText, msg)
		if err != nil {
			p.LogWarn("Failed to send message to client %s: %v", client.id, err)
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
				p.LogWarn("Failed to marshal message: %v", err)
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
		p.LogError(err.Error())
		return
	}

	client := &Client{
		conn: conn,
		id:   r.Header.Get("Sec-Websocket-Key"),
	}

	p.addClient(client)
	clients := p.totalClients()
	p.LogInfo(fmt.Sprintf("Joined size of connection pool: %v", clients))

	defer func() {
		p.removeClient(client)
		clients = p.totalClients()

		conn.Close(websocket.StatusNormalClosure, "")
		p.LogInfo(fmt.Sprintf("%s has disconnected. Remaining size of connection pool: %v", client.id, clients))
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
			p.LogError(err.Error())
			return
		}

		conn.Write(r.Context(), websocket.MessageText, msg)
	}
}
