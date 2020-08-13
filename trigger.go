package packer

import "time"

// TimeoutTrigger ...
func TimeoutTrigger(timeout time.Duration, p *Packer) {
	for {
		time.Sleep(timeout)
		nextSendTime := p.LastSendTime().Add(timeout)
		if time.Now().After(nextSendTime) {
			p.Send()
		}
	}
}
