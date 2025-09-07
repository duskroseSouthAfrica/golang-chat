package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type Message struct {
	Username  string    `json:"username"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type Client struct {
	conn     *websocket.Conn
	username string
	send     chan Message
	hub      *Hub
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan Message
	register   chan *Client
	unregister chan *Client
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// nolint:staticcheck // S1000 is ignored because multiplexing multiple channels is intentional
func (h *Hub) run() {
	log.Println("Hub is running")
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Printf("Client %s registered. Total: %d", client.username, len(h.clients))
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Printf("Client %s unregistered. Total: %d", client.username, len(h.clients))
			}
		case message := <-h.broadcast:
			log.Printf("Broadcasting: %s from %s to %d clients", message.Content, message.Username, len(h.clients))
			for client := range h.clients {
				select {
				case client.send <- message:
					log.Printf("Sent to %s", client.username)
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		var msg Message
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			log.Printf("Read error: %v", err)
			break
		}

		msg.Username = c.username
		msg.Timestamp = time.Now()
		log.Printf("Received from %s: %s", c.username, msg.Content)

		c.hub.broadcast <- msg
	}
}

func (c *Client) writePump() {
	defer c.conn.Close()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				log.Printf("Write error: %v", err)
				return
			}
			log.Printf("Sent message to %s", c.username)
		}
	}
}

func serveWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Upgrade error: %v", err)
		return
	}

	username := r.URL.Query().Get("username")
	if username == "" {
		username = fmt.Sprintf("User%d", time.Now().Unix()%1000)
	}

	client := &Client{
		hub:      hub,
		conn:     conn,
		username: username,
		send:     make(chan Message, 256),
	}

	client.hub.register <- client

	go client.writePump()
	go client.readPump()
}

func serveHome(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WorkChat Pro</title>
    <link href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.4.0/css/all.min.css" rel="stylesheet">
    <style>
    /* ... your full CSS here ... */
    </style>
</head>
<body>
    <!-- Login Screen -->
    <div id="loginOverlay" class="login-overlay">
        <div class="login-card">
            <div class="login-logo">
                <i class="fas fa-comments"></i>
            </div>
            <h1 class="login-title">Welcome to WorkChat Pro</h1>
            <p class="login-subtitle">Connect and collaborate with your team</p>
            <div class="login-form">
                <input type="text" id="usernameInput" class="login-input" placeholder="Enter your name" maxlength="20">
                <button onclick="connect()" class="login-btn">
                    <i class="fas fa-sign-in-alt"></i> Join Workspace
                </button>
            </div>
        </div>
    </div>

    <!-- Main Chat Interface -->
    <div id="chatContainer" class="chat-container" style="display: none;">
        <!-- Sidebar and main content... (rest of your HTML) -->
    </div>

    <script>
    // ... your full JS here ...
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	hub := newHub()
	go hub.run()

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		serveWS(hub, w, r)
	})

	fmt.Println("Chat server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
