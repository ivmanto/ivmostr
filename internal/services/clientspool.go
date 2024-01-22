package services

import "sync"

type ClientsPool struct {
	clpool *sync.Pool
	cnt    int
}

func NewClientsPool(size int) *ClientsPool {
	clpool := &sync.Pool{
		New: func() interface{} {
			return &Client{}
		},
	}

	return &ClientsPool{clpool, 0}
}

func (cp *ClientsPool) Get() *Client {
	cp.cnt++
	return cp.clpool.Get().(*Client)
}

// [ ]: Do more cleancing of the clnt...
func (cp *ClientsPool) Put(clnt *Client) {
	clnt.Conn = nil
	cp.clpool.Put(clnt)
	cp.cnt--
}

func (cp *ClientsPool) Len() int {
	return cp.cnt
}
