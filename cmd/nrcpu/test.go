package main

import (
	"fmt"
	"runtime"
	"sync"
)

func main() {
	fmt.Printf("Number of CPUs: %d\n", runtime.NumCPU())

	runtime.GOMAXPROCS(2)
	var wg sync.WaitGroup
	wg.Add(2)
	fmt.Printf("Starting\n")
	go func() {
		defer wg.Done()
		for n := 0; n < 10; n++ {
			fmt.Printf("%d\n", n)
		}
	}()
	go func() {
		defer wg.Done()
		for n := 100; n < 200; n++ {
			fmt.Printf("%d\n", n)
		}
	}()
	fmt.Println("Waiting...")
	wg.Wait()
	fmt.Println("Finish!")
}
