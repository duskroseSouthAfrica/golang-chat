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
<html>
<head>
    <title>Simple Chat</title>
    <style>
        body { font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; }
        .container { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        #messages { height: 400px; overflow-y: scroll; border: 1px solid #ccc; padding: 10px; margin-bottom: 20px; background: #fafafa; }
        .message { margin-bottom: 10px; padding: 8px; background: white; border-radius: 4px; }
        .username { font-weight: bold; color: #1976d2; }
        .timestamp { font-size: 0.8em; color: #666; float: right; }
        .input-group { display: flex; gap: 10px; }
        input { flex: 1; padding: 10px; border: 1px solid #ccc; border-radius: 4px; }
        button { padding: 10px 20px; background: #1976d2; color: white; border: none; border-radius: 4px; cursor: pointer; }
        #userSetup { margin-bottom: 20px; }
        .status { padding: 5px 10px; border-radius: 4px; margin-bottom: 10px; text-align: center; }
        .connected { background: #c8e6c9; color: #2e7d32; }
        .disconnected { background: #ffcdd2; color: #c62828; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Simple Chat</h1>
        <div id="status" class="status disconnected">Disconnected</div>
        
        <div id="userSetup">
            <div class="input-group">
                <input type="text" id="usernameInput" placeholder="Enter username">
                <button onclick="connect()">Join Chat</button>
            </div>
        </div>
        
        <div id="chatArea" style="display:none;">
            <div id="messages"></div>
            <div class="input-group">
                <input type="text" id="messageInput" placeholder="Type message...">
                <button onclick="sendMessage()">Send</button>
            </div>
        </div>
    </div>

    <script>
        let ws = null;
        let username = '';

        function connect() {
            username = document.getElementById('usernameInput').value.trim();
            if (!username) {
                alert('Please enter a username');
                return;
            }

            ws = new WebSocket('ws://localhost:8080/ws?username=' + username);
            
            ws.onopen = function() {
                document.getElementById('status').textContent = 'Connected as ' + username;
                document.getElementById('status').className = 'status connected';
                document.getElementById('userSetup').style.display = 'none';
                document.getElementById('chatArea').style.display = 'block';
            };
            
            ws.onmessage = function(event) {
                const message = JSON.parse(event.data);
                displayMessage(message);
            };
            
            ws.onclose = function() {
                document.getElementById('status').textContent = 'Disconnected';
                document.getElementById('status').className = 'status disconnected';
                document.getElementById('userSetup').style.display = 'block';
                document.getElementById('chatArea').style.display = 'none';
            };
        }

        function sendMessage() {
            const input = document.getElementById('messageInput');
            const content = input.value.trim();
            
            if (!content || !ws) return;
            
            ws.send(JSON.stringify({ content: content }));
            input.value = '';
        }

        function displayMessage(message) {
            const messagesDiv = document.getElementById('messages');
            const messageDiv = document.createElement('div');
            messageDiv.className = 'message';
            
            const timestamp = new Date(message.timestamp).toLocaleTimeString();
            messageDiv.innerHTML = 
                '<span class="timestamp">' + timestamp + '</span>' +
                '<span class="username">' + message.username + ':</span> ' +
                message.content;
            
            messagesDiv.appendChild(messageDiv);
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }

        // Enter to send
        document.addEventListener('keypress', function(e) {
            if (e.key === 'Enter') {
                if (document.getElementById('chatArea').style.display === 'none') {
                    connect();
                } else {
                    sendMessage();
                }
            }
        });
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