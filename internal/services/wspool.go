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
	mtx  sync.Mutex
}

func NewConnectionPool(size int) *ConnectionPool {
	pool := &sync.Pool{
		New: func() interface{} {
			return &Connection{}
		},
	}

	return &ConnectionPool{pool, sync.Mutex{}}
}

func (p *ConnectionPool) Get() *Connection {
	return p.pool.Get().(*Connection)
}

func (p *ConnectionPool) Put(conn *Connection) {
	p.mtx.Lock()
	defer p.mtx.Unlock()
	conn.WS = nil
	p.pool.Put(conn)
}
