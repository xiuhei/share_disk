package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// upgrader upgrades HTTP connections to WebSocket. Cross-origin browser
// requests are rejected by default; requests without an Origin header (native
// clients) are allowed. Configured trusted origins may be added later.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Non-browser client (native agent): allowed.
			return true
		}
		// Browser clients must be same-origin; there is no cross-origin use
		// case in V1 and allowing arbitrary origins enables cross-site
		// WebSocket hijacking.
		host := r.Host
		originHost := origin
		for _, prefix := range []string{"https://", "http://"} {
			originHost = trimPrefix(originHost, prefix)
		}
		return originHost == host
	},
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

// Message represents a WebSocket message
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Client represents a WebSocket client
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	userID   string
	deviceID string
}

// broadcastMessage is a message targeted at a user (empty userID = all users).
type broadcastMessage struct {
	userID string
	data   []byte
}

// directedMessage is a message targeted at a specific client (e.g. a pong).
type directedMessage struct {
	client *Client
	data   []byte
}

// Hub maintains the set of active clients and broadcasts messages. The clients
// map is owned exclusively by the Run goroutine: every send-channel write and
// every send-channel close happens there, so no mutex is required and a send
// channel can never be closed twice.
type Hub struct {
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan broadcastMessage
	deliver    chan directedMessage
}

// NewHub creates a new Hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan broadcastMessage),
		deliver:    make(chan directedMessage),
	}
}

// Run starts the hub. It is the sole owner of the client map.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}

		case msg := <-h.broadcast:
			for client := range h.clients {
				if msg.userID != "" && client.userID != msg.userID {
					continue
				}
				select {
				case client.send <- msg.data:
				default:
					// Slow client: drop and disconnect. Closing is done here and
					// the client is removed so a later unregister is a no-op.
					delete(h.clients, client)
					close(client.send)
				}
			}

		case msg := <-h.deliver:
			if _, ok := h.clients[msg.client]; ok {
				select {
				case msg.client.send <- msg.data:
				default:
					delete(h.clients, msg.client)
					close(msg.client.send)
				}
			}
		}
	}
}

// BroadcastToUser sends a message to all devices of a specific user.
func (h *Hub) BroadcastToUser(userID string, message *Message) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal message: %v", err)
		return
	}
	h.broadcast <- broadcastMessage{userID: userID, data: data}
}

// HandleWebSocket upgrades an already-authenticated request and registers the
// client. The caller is responsible for authentication before calling this.
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request, userID, deviceID string) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	client := &Client{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		userID:   userID,
		deviceID: deviceID,
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

// readPump pumps messages from the websocket connection. It never writes to the
// send channel directly; outbound traffic flows through the hub so that send
// channels have a single owner.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("Failed to unmarshal message: %v", err)
			continue
		}

		c.handleMessage(&msg)
	}
}

// writePump pumps messages from the hub to the websocket connection. It is the
// only goroutine that writes to the connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			if _, err := w.Write(message); err != nil {
				_ = w.Close()
				return
			}
			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage handles incoming WebSocket messages. Outbound replies are routed
// through the hub so the send channel remains single-owner.
func (c *Client) handleMessage(msg *Message) {
	switch msg.Type {
	case "ping":
		pong, _ := json.Marshal(&Message{Type: "pong"})
		c.hub.deliver <- directedMessage{client: c, data: pong}
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}

// NotificationService handles sending notifications
type NotificationService struct {
	hub *Hub
}

// NewNotificationService creates a new NotificationService
func NewNotificationService(hub *Hub) *NotificationService {
	return &NotificationService{hub: hub}
}

// NotifySyncEvent sends a sync event notification
func (s *NotificationService) NotifySyncEvent(userID string, seq int64, eventType string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"seq":        seq,
		"event_type": eventType,
	})

	msg := &Message{
		Type:    "sync_event",
		Payload: payload,
	}

	s.hub.BroadcastToUser(userID, msg)
}

// NotifyFileChange sends a file change notification
func (s *NotificationService) NotifyFileChange(userID, action, fileID string) {
	payload, _ := json.Marshal(map[string]interface{}{
		"action":  action,
		"file_id": fileID,
	})

	msg := &Message{
		Type:    "file_change",
		Payload: payload,
	}

	s.hub.BroadcastToUser(userID, msg)
}

// NotifyTransferUpdate sends a transfer update notification
func (s *NotificationService) NotifyTransferUpdate(userID, transferID, status string, progress float64) {
	payload, _ := json.Marshal(map[string]interface{}{
		"transfer_id": transferID,
		"status":      status,
		"progress":    progress,
	})

	msg := &Message{
		Type:    "transfer_update",
		Payload: payload,
	}

	s.hub.BroadcastToUser(userID, msg)
}
