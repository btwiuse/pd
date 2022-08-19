package swg

import (
	"os"
	"sync"
)

func (wg *WaitGroup) Cap() int {
	return cap(wg.ch)
}

func (wg *WaitGroup) Len() int {
	return len(wg.ch)
}

func (wg *WaitGroup) Add() {
	wg.WaitGroup.Add(1)
	wg.ch <- struct{}{}
	/*
	if wg.Len() + 1 >= wg.Limit {
		<-time.After(1000 * time.Millisecond)
	}
	*/
}

func (wg *WaitGroup) Done() {
	wg.WaitGroup.Done()
	<-wg.ch
}

func (wg *WaitGroup) Notify(sigs ...os.Signal) {
}

type WaitGroup struct {
	*sync.WaitGroup
	ch    chan struct{}
	Limit int
	sig   chan os.Signal
}

func New(j int) *WaitGroup {
	wg := &WaitGroup{
		ch:        make(chan struct{}, j),
		WaitGroup: &sync.WaitGroup{},
		Limit:     j,
		sig:       make(chan os.Signal, 1),
	}
	go wg.Notify()
	return wg
}
