package swg

import "sync"

func (wg *WaitGroup) Len() int {
	return len(wg.ch)
}

func (wg *WaitGroup) Add(){
	wg.WaitGroup.Add(1)
	wg.ch <- struct{}{}
}

func (wg *WaitGroup) Done(){
	<-wg.ch
	wg.WaitGroup.Done()
}

type WaitGroup struct {
	*sync.WaitGroup
	ch chan struct{}
}

func New(j int )(*WaitGroup) {
	return &WaitGroup{
		ch: make(chan struct{}, j),
		WaitGroup: &sync.WaitGroup{},
	}
}
