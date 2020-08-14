package packer

import "time"

// TimeoutTrigger calls packer.Send() method with a given interval.
func TimeoutTrigger(interval time.Duration, p *Packer) {
	for {
		time.Sleep(interval)
		nextSendTime := p.LastSendTime().Add(interval)
		if time.Now().After(nextSendTime) {
			p.Send()
		}
	}
}
