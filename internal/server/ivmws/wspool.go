package ivmws

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type connection struct {
	ws    *websocket.Conn
	mutex sync.Mutex
}

type wspool struct {
	connections []*connection
	mutex       sync.Mutex
}

func newPool(size int) *wspool {
	connections := make([]*connection, 0)
	for i := 0; i < size; i++ {
		connections = append(connections, newConnection())
	}
	return &wspool{connections: connections}
}

func (p *wspool) get() (*connection, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	for _, conn := range p.connections {
		if !conn.ws.IsAlive() {
			// Remove the dead connection from the pool
			p.connections = remove(p.connections, conn)

			// Create a new connection to replace the dead connection
			conn = newConnection()
			p.connections = append(p.connections, conn)

			return conn, nil
		}
	}

	return nil, fmt.Errorf("no idle connections available")
}

func (p *wspool) release(conn *connection) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	// Add the connection back to the pool
	p.connections = append(p.connections, conn)
}

func newConnection() *connection {
	ws, err := websocket.Upgrader{}.Upgrade(
		http.ResponseWriter{},
		http.Request{},
	)
	if err != nil {
		log.Fatal("Error upgrading WebSocket connection:", err)
	}

	return &connection{ws: ws}
}

func remove(connections []*connection, conn *connection) []*connection {
	for i, c := range connections {
		if c == conn {
			connections[i] = connections[len(connections)-1]
			connections[len(connections)-1] = nil
			connections = connections[:len(connections)-1]
			break
		}
	}

	return connections
}
