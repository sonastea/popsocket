package popsocket

import (
	"context"
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
	"github.com/sonastea/popsocket/pkg/config"
	"github.com/valkey-io/valkey-go"
)

const (

	// Time allowed to write a message to the client.
	writeWait = 10 * time.Second

	// Time allowed to read the next response from the client.
	readWait = 60 * time.Second

	// Time allowed for client to respond to the websocket.Conn.Ping(). Must be less than readWait
	heartbeatPeriod = (readWait * 9) / 10

	// Maximum message size allowed from client.
	MaxMessageSize = int64(1024)

	// writeTimeout sets the maximum duration before timing out writes of the response.
	writeTimeout = 15 * time.Second

	// readTimeout sets the maximum duration for reading the entire request.
	readTimeout = 15 * time.Second

	// idleTimeout sets the maximum amount of time to wait for the next request.
	idleTimeout = 60 * time.Second
)

var (
	AllowedOrigins []string

	_ PopSocketInterface = (*PopSocket)(nil)
)

type option func(ps *PopSocket) error

type PopSocketInterface interface {
	LogDebug(string, ...any)
	LogError(string, ...any)
	LogInfo(string, ...any)
	LogWarn(string, ...any)

	manageConnections(ctx context.Context)
	SetupRoutes(mux *http.ServeMux) error
	Start(ctx context.Context) error
}

type PopSocket struct {
	httpServer *http.Server
	logger     *slog.Logger
	mu         sync.RWMutex
	Valkey     valkey.Client

	broadcast  chan []byte
	clients    map[int32]map[string]client
	register   chan *Client
	unregister chan *Client

	MessageService
	SessionMiddleware
}

// init loads the app's environment variables and default
// allowed origins for the websocket connections.
func init() {
	config.LoadEnvVars()
	loadAllowedOrigins()
}

// loadAllowedOrigins gets and sets the the allowed origins from the ALLOWED_ORIGINS env variable.
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
func New(valkey valkey.Client, opts ...option) (*PopSocket, error) {
	ps := &PopSocket{
		broadcast:  make(chan []byte, MaxMessageSize),
		clients:    make(map[int32]map[string]client),
		register:   make(chan *Client),
		unregister: make(chan *Client),

		httpServer: &http.Server{
			Addr:         ":80",
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
			IdleTimeout:  idleTimeout,
			Handler:      http.NewServeMux(),
		},
		logger: newLogger(),
		Valkey: valkey,
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

// WithWriteTimeout allows setting a custom write timeout for the HTTP server.
func WithWriteTimeout(timeout time.Duration) option {
	return func(ps *PopSocket) error {
		ps.httpServer.WriteTimeout = timeout
		return nil
	}
}

// WithReadTimeout allows setting a custom read timeout for the HTTP server.
func WithReadTimeout(timeout time.Duration) option {
	return func(ps *PopSocket) error {
		ps.httpServer.ReadTimeout = timeout
		return nil
	}
}

// WithIdleTimeout allows setting a custom idle timeout for the HTTP server.
func WithIdleTimeout(timeout time.Duration) option {
	return func(ps *PopSocket) error {
		ps.httpServer.IdleTimeout = timeout
		return nil
	}
}

// WithMessageService allows passing a custom message store to PopSocket to utilize.
func WithMessageService(service MessageService) option {
	return func(ps *PopSocket) error {
		ps.MessageService = service
		return nil
	}
}

// WithSessionMiddleware allows passing a custom session middleware to PopSocket to utilize.
func WithSessionMiddleware(middleware SessionMiddleware) option {
	return func(ps *PopSocket) error {
		ps.SessionMiddleware = middleware
		return nil
	}
}

// Start launches the PopSocket HTTP server and manages graceful shutdown.
func (p *PopSocket) Start(ctx context.Context) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGINT, syscall.SIGKILL)
	defer cancel()

	go p.manageConnections(ctx)

	serverErrors := make(chan error, 1)

	go func() {
		p.logger.Info(fmt.Sprintf("Starting PopSocket server on %v", p.httpServer.Addr))
		if err := p.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErrors <- err
		}
	}()

	select {
	case <-ctx.Done():
		p.logger.Info(fmt.Sprintf("Shutting down PopSocket server..."))
		shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return p.httpServer.Shutdown(shutdownCtx)
	case err := <-serverErrors:
		cancel()
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

// totalClients returns the total number of connected clients including own multiple web connections.
func (p *PopSocket) totalClients() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.clients)
}

// manageConnections listens for client register/unregister events,
// message processing, and handles lifecycle events.
func (p *PopSocket) manageConnections(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return

		case _ = <-p.broadcast:

		case c := <-p.register:
			p.mu.Lock()
			if p.clients[c.ID()] == nil {
				p.clients[c.ID()] = make(map[string]client)
			}
			p.clients[c.ID()][c.ConnID()] = c
			p.mu.Unlock()
			p.LogInfo(fmt.Sprintf("Joined size of connection pool: %v", p.totalClients()))

		case c := <-p.unregister:
			if _, ok := p.clients[c.ID()][c.ConnID()]; ok {
				p.mu.Lock()
				close(c.send)
				if conns, ok := p.clients[c.ID()]; ok {
					delete(conns, c.ConnID())
					if len(conns) == 0 {
						delete(p.clients, c.ID())
					}
				}
				p.mu.Unlock()
				p.LogInfo(fmt.Sprintf("Left size of connection pool: %v", p.totalClients()))
			}

		}
	}
}

// serveWs handles incoming websocket connection requests.
func serveWs(p *PopSocket, w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: AllowedOrigins,
	})
	if err != nil {
		p.LogError("[ServeWs] ", "error", err.Error())
		return
	}

	client := newClient(r.Context(), r.Header.Get("Sec-Websocket-Key"), conn)
	client.Conn().SetReadLimit(MaxMessageSize)

	p.register <- client
	ctx, cancel := context.WithCancel(context.Background())

	go p.messageReceiver(ctx, client, cancel)
	go p.messageSender(ctx, client)
	go p.heartbeat(ctx, client, heartbeatPeriod)

	/* clientId := "1"
			hashKey := fmt.Sprintf("convosession:%s", clientId)
			err = p.Valkey.Do(r.Context(), p.Valkey.B().Hset().Key(hashKey).FieldValue().FieldValue("id", clientId).Build()).Error()
			if err != nil {
				p.LogError("Failed to set hash field: ", err)
			}
			keys := make(map[string]bool)
			cursor := uint64(0)
			results, _ := p.Valkey.Do(r.Context(), p.Valkey.B().Scan().Cursor(cursor).Match("convosession:*").Count(100).Build()).AsScanEntry()

			for _, key := range results.Elements {
				keys[key] = true
			}

	    cursor = nextCursor
			fmt.Printf("%+v \n", keys) */
}

// SetupRoutes registers the http routes for the PopSocket server.
func (p *PopSocket) SetupRoutes(mux *http.ServeMux) error {
	mux.HandleFunc("/", p.ValidateCookie(func(w http.ResponseWriter, r *http.Request) {
		serveWs(p, w, r)
	}))

	return nil
}

// heartbeat sends periodic ping messages to the client to ensure the connection is still active.
func (p *PopSocket) heartbeat(ctx context.Context, client *Client, period time.Duration) {
	ticker := time.NewTimer(period)

	defer func() {
		ticker.Stop()
		client.conn.Close(websocket.StatusNormalClosure, "Heartbeat gone.")
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// TODO: Validate session has not expired, close connection if that's not the case.
			err := client.conn.Ping(ctx)
			if err != nil {
				p.LogInfo(fmt.Sprintf("[%s] ping error: %+v \n", time.Now().Local(), err))
				client.conn.Close(websocket.StatusPolicyViolation, "Pong not received.")
				return
			}
			ticker.Reset(period)
			p.LogInfo(fmt.Sprintf("Sent heartbeat to userID %d, connID %s", client.ID(), client.ConnID()))
		}
	}
}
