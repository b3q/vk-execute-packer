package packer

import (
	"sync"
)

type tokenPool struct {
	tmap   map[string]struct{}
	tokens chan string
	mtx    sync.RWMutex
}

func newTokenPool(tokens ...string) *tokenPool {
	c := make(chan string, len(tokens))
	tmap := make(map[string]struct{})
	for _, t := range tokens {
		c <- t
		tmap[t] = struct{}{}
	}
	return &tokenPool{
		tokens: c,
		tmap:   tmap,
	}
}

func (tp *tokenPool) append(token string) {
	tp.mtx.Lock()
	defer tp.mtx.Unlock()
	if _, found := tp.tmap[token]; found {
		return
	}

	newChan := make(chan string, len(tp.tokens)+1)
	newChan <- token
	close(tp.tokens)
	for t := range tp.tokens {
		newChan <- t
	}

	tp.tmap[token] = struct{}{}
	tp.tokens = newChan
	return
}

func (tp *tokenPool) get() string {
	tp.mtx.RLock()
	defer tp.mtx.RUnlock()
	token := <-tp.tokens
	tp.tokens <- token
	return token
}

func (tp *tokenPool) len() int {
	tp.mtx.RLock()
	defer tp.mtx.RUnlock()
	return len(tp.tmap)
}
