package breaker

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type Count struct {
	val      uint32
	timeTamp int64
}

const (
	STATUS_CLOSED int32 = iota
	STATUS_OPEN
)

var (
	cellCnt = 20
)

/*
滑动窗口计数器
*/
type SlidingWindow struct {
	/*滑动窗口周期*/
	windowSize int64
	/*熔断器关闭请求滑动窗口计数器*/
	resqRingWindow []*Count
	/*熔断器关闭失败请求滑动窗口计数器*/
	failRingWindow []*Count
	/*熔断器关闭计数通道*/
	reaChan chan bool
	/*半开启计数请求*/
	breakReq int32
	/*半开启计数错误*/
	breakFail int32
	/*熔断恢复需要请求数*/
	breakCnt int32
	/*起始时间*/
	startTime int64
	/*格子时间*/
	diff      int64
	/*错误率*/
	ErrorPercent int
	/*半开启错误率*/
	BreakErrorPercent int
	/*状态*/
	status int32
}

type SlidingWindowSetting struct {
	/*周期*/
	CycleTime int64
	/*错误率*/
	ErrorPercent int
	/*半开启错误率*/
	BreakErrorPercent int
	/*半开启错误率*/
	BreakCnt int
}

func (this *SlidingWindow) consumeRes() {
	for {
		select {
		case res := <-this.reaChan:
			if res {
				this.add()
			} else {
				this.addfail()
			}
		}
	}
}

func NewSlidingWindow(c SlidingWindowSetting) *SlidingWindow {
	rcs := []*Count{}
	fcs := []*Count{}
	for idx := 0; idx < int(cellCnt); idx++ {
		c := &Count{}
		c.val = 0
		c.timeTamp = 0
		rcs = append(rcs, c)
	}
	for idx := 0; idx < int(cellCnt); idx++ {
		c := &Count{}
		c.val = 0
		c.timeTamp = 0
		fcs = append(fcs, c)
	}
	slidingWindow := &SlidingWindow{
		windowSize:        c.CycleTime,
		diff:              c.CycleTime / int64(cellCnt),
		resqRingWindow:    rcs,
		failRingWindow:    fcs,
		startTime:         0,
		reaChan:           make(chan bool, 10000),
		ErrorPercent:      c.ErrorPercent,
		BreakErrorPercent: c.BreakErrorPercent,
		breakCnt:          int32(c.BreakCnt),
	}
	go slidingWindow.consumeRes()
	return slidingWindow
}

func (this *SlidingWindow) Add(res bool) {
	this.reaChan <- res
}

func (this *SlidingWindow) AddBreak(res bool) bool {
	if res {
		reqTotal := atomic.AddInt32(&this.breakReq, 1)
		fmt.Println("AddBreak reqTotal", reqTotal)
		if reqTotal >= this.breakCnt {
			defer this.clear()
			failTotal := atomic.LoadInt32(&this.breakFail)
			if int(float32(failTotal)/float32(reqTotal)*100+0.5) >= this.BreakErrorPercent {
				atomic.StoreInt32(&this.status, STATUS_OPEN)
			} else {
				atomic.StoreInt32(&this.status, STATUS_CLOSED)
			}
			return true
		}
		return false
	}
	reqTotal := atomic.AddInt32(&this.breakReq, 1)
	failTotal := atomic.AddInt32(&this.breakFail, 1)
	if reqTotal >= this.breakCnt {
		defer this.clear()
		if int(float32(failTotal)/float32(reqTotal)*100+0.5) >= this.BreakErrorPercent {
			atomic.StoreInt32(&this.status, STATUS_OPEN)
		} else {
			atomic.StoreInt32(&this.status, STATUS_CLOSED)
		}
		return true
	}
	return false
}

/*计算错误率*/
func (this *SlidingWindow) getFailPercentThreshold() (bool, int) {
	bucket := time.Now().Local().Unix() - 10

	var reqTotalCnt uint32 = 0
	var failTotalCnt uint32 = 0
	reqTotalCntChan := make(chan uint32)
	failTotalCntChan := make(chan uint32)
	go func() {
		var reqTotalCnt uint32 = 0
		for _, count := range this.resqRingWindow {
			if count.timeTamp <= bucket {
				continue
			}
			reqTotalCnt += count.val
		}
		reqTotalCntChan <- reqTotalCnt
	}()

	go func() {
		var failTotalCnt uint32 = 0
		for _, count := range this.failRingWindow {
			if count.timeTamp <= bucket {
				continue
			}
			failTotalCnt += count.val

		}
		failTotalCntChan <- failTotalCnt
	}()

	time := 0
loop:
	for {
		select {
		case reqTotalCnt = <-reqTotalCntChan:
			time++
			if time >= 2 {
				break loop
			}
		case failTotalCnt = <-failTotalCntChan:
			time++
			if time >= 2 {
				break loop
			}
		}
	}
	if reqTotalCnt < 10 {
		return false, 0
	}
	p := int(float32(failTotalCnt)/float32(reqTotalCnt)*100 + 0.5)
	fmt.Println("failTotalCnt", failTotalCnt, "reqTotalCnt", reqTotalCnt, p)
	return true, p
}

/*
添加成功计数方法，因为加锁所以默认所有请求是有时序性的
*/
func (this *SlidingWindow) add() {
	addTime := time.Now().Local().Unix()
	diffTime := addTime - this.startTime
	if diffTime >= this.windowSize {
		this.startTime = addTime
		diffTime = 0
	}
	loc := diffTime / this.diff

	if addTime-this.resqRingWindow[loc].timeTamp >= this.windowSize {
		this.resqRingWindow[loc].val = 1
		this.resqRingWindow[loc].timeTamp = addTime
		return
	}
	this.resqRingWindow[loc].val++
}

func (this *SlidingWindow) addfail() {
	addTime := time.Now().Local().Unix()
	diffTime := addTime - this.startTime
	if diffTime >= this.windowSize {
		this.startTime = addTime
		diffTime = 0
	}
	loc := diffTime / this.diff
	var wg sync.WaitGroup
	/*请求计数*/
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		if addTime-this.resqRingWindow[loc].timeTamp >= this.windowSize {
			this.resqRingWindow[loc].val = 1
			this.resqRingWindow[loc].timeTamp = addTime
			return
		}
		this.resqRingWindow[loc].val++
	}(&wg)
	/*失败计数*/
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		if addTime-this.failRingWindow[loc].timeTamp >= this.windowSize {
			this.failRingWindow[loc].val = 1
			this.failRingWindow[loc].timeTamp = addTime
			return
		}
		this.failRingWindow[loc].val++
	}(&wg)
	wg.Wait()

	ok, percent := this.getFailPercentThreshold()
	if !ok {
		return
	}
	if percent >= this.ErrorPercent {
		atomic.StoreInt32(&this.status, STATUS_OPEN)
	}
}

func (this *SlidingWindow) clear() {
	atomic.StoreInt32(&this.breakReq, 0)
	atomic.StoreInt32(&this.breakFail, 0)
}

func (this *SlidingWindow) GetStatus() int32 {
	return atomic.LoadInt32(&this.status)
}
