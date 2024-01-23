package services

import (
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
			return &Connection{}
		},
	}

	return &ConnectionPool{pool}
}

func (p *ConnectionPool) Get() *Connection {
	return p.pool.Get().(*Connection)
}

func (p *ConnectionPool) Put(conn *Connection) {
	conn = nil
	p.pool.Put(conn)
}
