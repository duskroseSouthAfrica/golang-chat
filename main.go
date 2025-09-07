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
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>WorkChat Pro</title>
    <link href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.4.0/css/all.min.css" rel="stylesheet">
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            height: 100vh;
            overflow: hidden;
        }

        .chat-container {
            display: flex;
            height: 100vh;
            background: white;
            margin: 20px;
            border-radius: 16px;
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.15);
            overflow: hidden;
        }

        /* Sidebar */
        .sidebar {
            width: 280px;
            background: linear-gradient(180deg, #2c3e50 0%, #34495e 100%);
            display: flex;
            flex-direction: column;
            border-radius: 16px 0 0 16px;
        }

        .logo {
            padding: 24px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .logo h1 {
            color: white;
            font-size: 24px;
            font-weight: 700;
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .logo i {
            color: #3498db;
            font-size: 28px;
        }

        .user-info {
            padding: 20px 24px;
            border-bottom: 1px solid rgba(255, 255, 255, 0.1);
        }

        .user-avatar {
            width: 48px;
            height: 48px;
            border-radius: 50%;
            background: linear-gradient(135deg, #3498db, #2980b9);
            display: flex;
            align-items: center;
            justify-content: center;
            color: white;
            font-weight: bold;
            font-size: 18px;
            margin-bottom: 12px;
        }

        .user-name {
            color: white;
            font-weight: 600;
            font-size: 16px;
            margin-bottom: 4px;
        }

        .user-status {
            display: flex;
            align-items: center;
            gap: 8px;
            color: #95a5a6;
            font-size: 14px;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #2ecc71;
            animation: pulse 2s infinite;
        }

        @keyframes pulse {
            0% { transform: scale(1); opacity: 1; }
            50% { transform: scale(1.2); opacity: 0.8; }
            100% { transform: scale(1); opacity: 1; }
        }

        .channels {
            flex: 1;
            padding: 20px 0;
        }

        .channel-header {
            padding: 0 24px 12px;
            color: #95a5a6;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
            letter-spacing: 1px;
        }

        .channel {
            padding: 12px 24px;
            color: #bdc3c7;
            cursor: pointer;
            transition: all 0.3s ease;
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .channel:hover {
            background: rgba(255, 255, 255, 0.1);
            color: white;
        }

        .channel.active {
            background: rgba(52, 152, 219, 0.2);
            color: #3498db;
            border-right: 3px solid #3498db;
        }

        /* Main Chat Area */
        .main-content {
            flex: 1;
            display: flex;
            flex-direction: column;
            background: #f8f9fa;
        }

        /* Chat Header */
        .chat-header {
            background: white;
            padding: 20px 32px;
            border-bottom: 1px solid #e9ecef;
            display: flex;
            align-items: center;
            justify-content: space-between;
        }

        .chat-title {
            display: flex;
            align-items: center;
            gap: 12px;
        }

        .chat-title h2 {
            color: #2c3e50;
            font-size: 20px;
            font-weight: 600;
        }

        .chat-title .channel-icon {
            color: #6c757d;
        }

        .chat-actions {
            display: flex;
            gap: 16px;
        }

        .action-btn {
            background: none;
            border: none;
            color: #6c757d;
            font-size: 18px;
            cursor: pointer;
            padding: 8px;
            border-radius: 50%;
            transition: all 0.3s ease;
        }

        .action-btn:hover {
            background: #f1f3f4;
            color: #495057;
        }

        /* Messages Area */
        .messages-container {
            flex: 1;
            overflow-y: auto;
            padding: 24px 32px;
            scroll-behavior: smooth;
        }

        .message {
            display: flex;
            margin-bottom: 24px;
            animation: messageSlide 0.3s ease-out;
        }

        @keyframes messageSlide {
            from {
                opacity: 0;
                transform: translateY(20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        .message-avatar {
            width: 40px;
            height: 40px;
            border-radius: 50%;
            background: linear-gradient(135deg, #e74c3c, #c0392b);
            display: flex;
            align-items: center;
            justify-content: center;
            color: white;
            font-weight: bold;
            margin-right: 12px;
            flex-shrink: 0;
        }

        .message-content {
            flex: 1;
            background: white;
            border-radius: 12px;
            padding: 16px;
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
            position: relative;
        }

        .message-content::before {
            content: '';
            position: absolute;
            left: -8px;
            top: 20px;
            width: 0;
            height: 0;
            border-top: 8px solid transparent;
            border-bottom: 8px solid transparent;
            border-right: 8px solid white;
        }

        .message-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 8px;
        }

        .message-username {
            font-weight: 600;
            color: #2c3e50;
            font-size: 14px;
        }

        .message-time {
            color: #95a5a6;
            font-size: 12px;
        }

        .message-text {
            color: #495057;
            line-height: 1.5;
            word-wrap: break-word;
        }

        /* My messages */
        .message.own {
            flex-direction: row-reverse;
        }

        .message.own .message-avatar {
            margin-left: 12px;
            margin-right: 0;
            background: linear-gradient(135deg, #3498db, #2980b9);
        }

        .message.own .message-content {
            background: linear-gradient(135deg, #3498db, #2980b9);
            color: white;
        }

        .message.own .message-content::before {
            left: auto;
            right: -8px;
            border-left: 8px solid #3498db;
            border-right: none;
        }

        .message.own .message-username {
            color: rgba(255, 255, 255, 0.9);
        }

        .message.own .message-time {
            color: rgba(255, 255, 255, 0.7);
        }

        .message.own .message-text {
            color: white;
        }

        /* System messages */
        .system-message {
            text-align: center;
            margin: 16px 0;
            padding: 8px 16px;
            background: rgba(52, 152, 219, 0.1);
            border-radius: 20px;
            color: #3498db;
            font-size: 13px;
            font-style: italic;
        }

        /* Input Area */
        .input-container {
            background: white;
            padding: 24px 32px;
            border-top: 1px solid #e9ecef;
        }

        .input-wrapper {
            display: flex;
            align-items: end;
            gap: 16px;
            background: #f8f9fa;
            border-radius: 24px;
            padding: 12px 20px;
            border: 2px solid transparent;
            transition: all 0.3s ease;
        }

        .input-wrapper:focus-within {
            border-color: #3498db;
            background: white;
            box-shadow: 0 4px 12px rgba(52, 152, 219, 0.15);
        }

        #messageInput {
            flex: 1;
            border: none;
            background: transparent;
            font-size: 16px;
            color: #495057;
            resize: none;
            outline: none;
            max-height: 120px;
            min-height: 20px;
            line-height: 1.4;
        }

        #messageInput::placeholder {
            color: #adb5bd;
        }

        .send-btn {
            background: linear-gradient(135deg, #3498db, #2980b9);
            border: none;
            border-radius: 50%;
            width: 48px;
            height: 48px;
            color: white;
            cursor: pointer;
            transition: all 0.3s ease;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 18px;
        }

        .send-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 8px 20px rgba(52, 152, 219, 0.4);
        }

        .send-btn:disabled {
            background: #bdc3c7;
            cursor: not-allowed;
            transform: none;
            box-shadow: none;
        }

        /* Login Screen */
        .login-overlay {
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            display: flex;
            align-items: center;
            justify-content: center;
            z-index: 1000;
        }

        .login-card {
            background: white;
            border-radius: 20px;
            padding: 48px;
            box-shadow: 0 30px 80px rgba(0, 0, 0, 0.2);
            text-align: center;
            max-width: 400px;
            width: 90%;
            animation: loginSlide 0.5s ease-out;
        }

        @keyframes loginSlide {
            from {
                opacity: 0;
                transform: translateY(50px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        .login-logo {
            color: #3498db;
            font-size: 64px;
            margin-bottom: 24px;
        }

        .login-title {
            font-size: 28px;
            font-weight: 700;
            color: #2c3e50;
            margin-bottom: 12px;
        }

        .login-subtitle {
            color: #6c757d;
            margin-bottom: 32px;
            font-size: 16px;
        }

        .login-form {
            display: flex;
            flex-direction: column;
            gap: 20px;
        }

        .login-input {
            padding: 16px 20px;
            border: 2px solid #e9ecef;
            border-radius: 12px;
            font-size: 16px;
            transition: all 0.3s ease;
        }

        .login-input:focus {
            outline: none;
            border-color: #3498db;
            box-shadow: 0 4px 12px rgba(52, 152, 219, 0.15);
        }

        .login-btn {
            background: linear-gradient(135deg, #3498db, #2980b9);
            color: white;
            border: none;
            padding: 16px;
            border-radius: 12px;
            font-size: 16px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s ease;
        }

        .login-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 8px 25px rgba(52, 152, 219, 0.4);
        }

        /* Responsive Design */
        @media (max-width: 768px) {
            .chat-container {
                margin: 0;
                border-radius: 0;
                height: 100vh;
            }
            
            .sidebar {
                width: 240px;
            }
            
            .messages-container {
                padding: 16px 20px;
            }
            
            .input-container {
                padding: 16px 20px;
            }
        }

        /* Custom Scrollbar */
        .messages-container::-webkit-scrollbar {
            width: 6px;
        }

        .messages-container::-webkit-scrollbar-track {
            background: #f1f1f1;
            border-radius: 3px;
        }

        .messages-container::-webkit-scrollbar-thumb {
            background: #c1c1c1;
            border-radius: 3px;
        }

        .messages-container::-webkit-scrollbar-thumb:hover {
            background: #a8a8a8;
        }
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
        <!-- Sidebar -->
        <div class="sidebar">
            <div class="logo">
                <h1><i class="fas fa-comments"></i> WorkChat Pro</h1>
            </div>
            
            <div class="user-info">
                <div class="user-avatar" id="userAvatar">U</div>
                <div class="user-name" id="userName">User</div>
                <div class="user-status">
                    <div class="status-dot"></div>
                    Online
                </div>
            </div>
            
            <div class="channels">
                <div class="channel-header">Channels</div>
                <div class="channel active">
                    <i class="fas fa-hashtag"></i> general
                </div>
                <div class="channel">
                    <i class="fas fa-hashtag"></i> random
                </div>
                <div class="channel">
                    <i class="fas fa-lock"></i> private
                </div>
            </div>
        </div>

        <!-- Main Content -->
        <div class="main-content">
            <div class="chat-header">
                <div class="chat-title">
                    <i class="fas fa-hashtag channel-icon"></i>
                    <h2>general</h2>
                </div>
                <div class="chat-actions">
                    <button class="action-btn" title="Search"><i class="fas fa-search"></i></button>
                    <button class="action-btn" title="Call"><i class="fas fa-phone"></i></button>
                    <button class="action-btn" title="Settings"><i class="fas fa-cog"></i></button>
                </div>
            </div>

            <div class="messages-container" id="messages">
                <!-- Messages will be inserted here -->
            </div>

            <div class="input-container">
                <div class="input-wrapper">
                    <textarea id="messageInput" placeholder="Type your message..." rows="1"></textarea>
                    <button class="send-btn" onclick="sendMessage()" id="sendBtn">
                        <i class="fas fa-paper-plane"></i>
                    </button>
                </div>
            </div>
        </div>
    </div>

    <script>
        let ws = null;
        let username = '';
        let currentUser = '';

        function connect() {
            const input = document.getElementById('usernameInput');
            username = input.value.trim();
            
            if (!username) {
                input.focus();
                return;
            }

            currentUser = username;
            ws = new WebSocket('ws://localhost:8080/ws?username=' + encodeURIComponent(username));
            
            ws.onopen = function() {
                document.getElementById('loginOverlay').style.display = 'none';
                document.getElementById('chatContainer').style.display = 'flex';
                
                // Update user info in sidebar
                document.getElementById('userName').textContent = username;
                document.getElementById('userAvatar').textContent = username.charAt(0).toUpperCase();
                
                // Focus message input
                document.getElementById('messageInput').focus();
            };
            
            ws.onmessage = function(event) {
                const message = JSON.parse(event.data);
                displayMessage(message);
            };
            
            ws.onclose = function() {
                document.getElementById('loginOverlay').style.display = 'flex';
                document.getElementById('chatContainer').style.display = 'none';
            };

            ws.onerror = function(error) {
                alert('Connection failed. Please try again.');
            };
        }

        function sendMessage() {
            const input = document.getElementById('messageInput');
            const content = input.value.trim();
            
            if (!content || !ws || ws.readyState !== WebSocket.OPEN) return;
            
            ws.send(JSON.stringify({ content: content }));
            input.value = '';
            adjustTextareaHeight(input);
        }

        function displayMessage(message) {
            const messagesDiv = document.getElementById('messages');
            
            // Check if it's a system message
            if (message.username === 'System') {
                const systemDiv = document.createElement('div');
                systemDiv.className = 'system-message';
                systemDiv.textContent = message.content;
                messagesDiv.appendChild(systemDiv);
            } else {
                const messageDiv = document.createElement('div');
                messageDiv.className = message.username === currentUser ? 'message own' : 'message';
                
                const avatar = document.createElement('div');
                avatar.className = 'message-avatar';
                avatar.textContent = message.username.charAt(0).toUpperCase();
                
                const content = document.createElement('div');
                content.className = 'message-content';
                
                const header = document.createElement('div');
                header.className = 'message-header';
                
                const usernameSpan = document.createElement('span');
                usernameSpan.className = 'message-username';
                usernameSpan.textContent = message.username;
                
                const timeSpan = document.createElement('span');
                timeSpan.className = 'message-time';
                timeSpan.textContent = new Date(message.timestamp).toLocaleTimeString();
                
                const textDiv = document.createElement('div');
                textDiv.className = 'message-text';
                textDiv.textContent = message.content;
                
                header.appendChild(usernameSpan);
                header.appendChild(timeSpan);
                content.appendChild(header);
                content.appendChild(textDiv);
                
                messageDiv.appendChild(avatar);
                messageDiv.appendChild(content);
                messagesDiv.appendChild(messageDiv);
            }
            
            messagesDiv.scrollTop = messagesDiv.scrollHeight;
        }

        function adjustTextareaHeight(textarea) {
            textarea.style.height = 'auto';
            textarea.style.height = Math.min(textarea.scrollHeight, 120) + 'px';
        }

        // Event listeners
        document.addEventListener('DOMContentLoaded', function() {
            const messageInput = document.getElementById('messageInput');
            const usernameInput = document.getElementById('usernameInput');
            
            messageInput.addEventListener('input', function() {
                adjustTextareaHeight(this);
            });

            messageInput.addEventListener('keydown', function(e) {
                if (e.key === 'Enter' && !e.shiftKey) {
                    e.preventDefault();
                    sendMessage();
                }
            });

            usernameInput.addEventListener('keydown', function(e) {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    connect();
                }
            });

            // Auto-focus username input
            usernameInput.focus();
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