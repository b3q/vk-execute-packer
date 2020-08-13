package packer

import "time"

// TimeoutTrigger ...
func TimeoutTrigger(timeout time.Duration, p *Packer) {
	for {
		time.Sleep(timeout)
		nextFlushTime := p.LastFlushTime().Add(timeout)
		if time.Now().After(nextFlushTime) {
			p.Flush()
		}
	}
}
