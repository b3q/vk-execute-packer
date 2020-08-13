package packer

import (
	"log"
	"time"
)

// Flusher interface
type Flusher func(p *Packer)

// TimeoutBasedFlusher ...
func TimeoutBasedFlusher(timeout time.Duration) Flusher {
	return func(p *Packer) {
	loop:
		time.Sleep(timeout)
		nextFlushTime := p.LastFlushTime().Add(timeout)
		if time.Now().After(nextFlushTime) {
			if p.debug {
				log.Println("flusher: timeout")
			}
			p.Flush()
			goto loop
		}

		if p.debug {
			log.Println("flusher: skipping")
		}

		goto loop
	}
}
