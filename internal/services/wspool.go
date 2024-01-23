package services

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type Connection struct {
	WS *websocket.Conn
}

type ConnectionPool struct {
	pool *sync.Pool
}

func NewConnectionPool(size int) *ConnectionPool {
	pool := &sync.Pool{
		New: func() interface{} {
			return make([]*Connection, size)
		},
	}

	return &ConnectionPool{pool}
}

func (p *ConnectionPool) Get() *Connection {
	return p.pool.Get().(*Connection)
}

func (p *ConnectionPool) Put(conn *Connection) {
	e := conn.WS.Close()
	if e != nil {
		log.Println("Error closing connection.Ws:", e)
	}
	p.pool.Put(conn)
}
