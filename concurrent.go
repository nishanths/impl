package main

import "sync"

// converge forwards into out each value received from the supplied channels.
// out is closed once all values have been sent.
func converge(chs []<-chan ObjectIdent) (out <-chan ObjectIdent) {
	var wg sync.WaitGroup
	wg.Add(len(chs))
	x := make(chan ObjectIdent)

	for _, c := range chs {
		go func(c <-chan ObjectIdent) {
			defer wg.Done()
			for val := range c {
				x <- val
			}
		}(c)
	}

	go func() {
		wg.Wait()
		close(x)
	}()

	return x
}
