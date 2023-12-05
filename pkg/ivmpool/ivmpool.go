package ivmpool

type GoroutinePool struct {
	workerChan chan chan interface{}
	quitChan   chan struct{}
}

func NewGoroutinePool(workerCount int) *GoroutinePool {
	pool := &GoroutinePool{
		workerChan: make(chan chan interface{}, workerCount),
		quitChan:   make(chan struct{}),
	}

	for i := 0; i < workerCount; i++ {
		worker := func() {
			for {
				workerChannel := <-pool.workerChan

				go func() {
					defer close(workerChannel)

					for task := range workerChannel {
						task.(func())()
					}
				}()
			}
		}

		go worker()
	}

	return pool
}

func (pool *GoroutinePool) Schedule(task func()) {
	workerChannel := <-pool.workerChan
	defer func() {
		pool.workerChan <- workerChannel
	}()

	workerChannel <- task
}

func (pool *GoroutinePool) Close() {
	close(pool.quitChan)
}
