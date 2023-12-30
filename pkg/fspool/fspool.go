// fspool creates and manage a Firestore database conections pool.
package fspool

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
)

const (
	poolSize = 10
)

var (
	mu           sync.Mutex
	busy_clients = []*firestore.Client{}
)

type ConnectionPool struct {
	// The map key will be the client memory address
	clients map[*firestore.Client]firestore.Client
}

func NewConnectionPool(prj string) *ConnectionPool {
	if prj == "" {
		return nil
	}

	pool := &ConnectionPool{}
	pool.clients = make(map[*firestore.Client]firestore.Client, poolSize)

	for i := 0; i < poolSize; i++ {
		cnt := 0
		ctx := context.Background()
		client, err := firestore.NewClient(ctx, prj)
		if err != nil {
			if cnt++; cnt > 10 {
				log.Fatalf("Failed to create Firestore client: %v", err)
				break
			}

			log.Fatalf("Failed to create Firestore client (retry %d): %v", cnt, err)
			i--
			continue
		}

		mu.Lock()
		pool.clients[client] = *client
		mu.Unlock()
	}

	return pool
}

func (pool *ConnectionPool) GetClient() (*firestore.Client, error) {

	for ma, client := range pool.clients {

		if containsMA(busy_clients, ma) {
			continue
		}

		busy_clients = append(busy_clients, &client)
		return &client, nil

	}

	return nil, fmt.Errorf("No available clients in pool")
}

func (pool *ConnectionPool) ReleaseClient(client *firestore.Client) {

	for {
		err := client.Close()
		if err != nil {
			fmt.Printf("Failed to close Firestore client [%v]: %v", client, err)
			time.Sleep(time.Second * 1)
			continue
		}
		break
	}

	// Remove from busy clients
	for i, ma := range busy_clients {
		if ma == client {
			mu.Lock()
			busy_clients = append(busy_clients[:i], busy_clients[i+1:]...)
			mu.Unlock()
			break
		}
	}

}

// ContainsMA - veryfies if an array of memmory addresses cotains a specific memmory address
func containsMA(slice []*firestore.Client, element *firestore.Client) bool {
	for _, item := range slice {
		if item == element {
			return true
		}
	}
	return false
}
