package breaker

import "sync"

type limitPoolManager struct {
	max     int
	tickets chan *struct{}
	lock    *sync.RWMutex
}

/*
方法返回一个限流器
*/
func NewLimitPoolManager(max int) *limitPoolManager {
	lpm := new(limitPoolManager)
	tickets := make(chan *struct{}, max)
	for i := 0; i < max; i++ {
		tickets <- &struct{}{}
	}
	lpm.max = max
	lpm.tickets = tickets
	lpm.lock = &sync.RWMutex{}
	return lpm
}

/*
方法填充限流器所有令牌
*/
func (this *limitPoolManager) ReturnAll() {
	this.lock.Lock()
	defer this.lock.Unlock()
	if len(this.tickets) == 0 {
		for i := 0; i < this.max; i++ {
			this.tickets <- &struct{}{}
		}
	}
}

/*
方法返回一个令牌，得到令牌返回true，令牌用完后返回false
*/
func (this *limitPoolManager) GetTicket() bool {
	this.lock.RLock()
	defer this.lock.RUnlock()
	select {
	case <-this.tickets:
		return true
	default:
		return false
	}
}

/*
方法返回剩余令牌数
*/
func (this *limitPoolManager) GetRemaind() int {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return len(this.tickets)
}
