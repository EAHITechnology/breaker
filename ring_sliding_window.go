package breaker

import (
	"sync"
	"time"
)

type Count struct {
	val      uint32
	timeTamp int64
}

/*
滑动窗口计数器,小周期为秒
*/
type SlidingWindow struct {
	/*滑动窗口周期*/
	windowSize int64
	/*请求滑动窗口计数器*/
	resqRingWindow []*Count
	/*尾指针*/
	pEnd int
	/*头指针*/
	pStart int
	/*起始时间*/
	startTime int64
	/*读写锁*/
	lock *sync.RWMutex
}

type SlidingWindowSetting struct {
	/*周期*/
	CycleTime int64
}

func NewSlidingWindow(c SlidingWindowSetting) *SlidingWindow {
	startTime := time.Now().Local().Unix()
	rcs := []*Count{}
	for idx := 0; idx < int(c.CycleTime); idx++ {
		c := &Count{}
		c.val = 0
		c.timeTamp = startTime + int64(idx)
		rcs = append(rcs, c)
	}
	return &SlidingWindow{
		windowSize:     c.CycleTime,
		resqRingWindow: rcs,
		startTime:      startTime,
		pEnd:           int(c.CycleTime) - 1,
		pStart:         0,
		lock:           &sync.RWMutex{},
	}
}

/*
添加成功计数方法，因为加锁所以默认所有请求是有时序性的
*/
func (this *SlidingWindow) Add() {
	this.lock.Lock()
	defer this.lock.Unlock()
	addTime := time.Now().Local().Unix()

	/*时间间隔*/
	diff := addTime - this.startTime
	/*如果间隔大于整个窗口时间则头尾指针向后*/
	if diff >= this.windowSize {
		/*判断是否移动尾指针*/
		if this.resqRingWindow[this.pEnd].timeTamp >= addTime {
			this.resqRingWindow[this.pEnd].val++
			return
		}
		/*移动尾指针*/
		this.pEnd++
		if this.pEnd >= int(this.windowSize) {
			this.pEnd = 0
		}
		/*尾指针数据更新*/
		this.resqRingWindow[this.pEnd].timeTamp = addTime
		this.resqRingWindow[this.pEnd].val = 1

		/*移动头指针*/
		this.pStart++
		if this.pStart >= int(this.windowSize) {
			this.pStart = 0
		}
		if this.resqRingWindow[this.pStart].timeTamp != 0 {
			this.startTime = this.resqRingWindow[this.pStart].timeTamp
			return
		}
		this.startTime++
		this.resqRingWindow[this.pStart].timeTamp = this.startTime
		return
	} else {
		/*这部分为了初始数组不全和diff==0的情况*/
		location := int64(this.pStart) + diff
		if location >= this.windowSize {
			location = location - this.windowSize
		}
		if this.resqRingWindow[location].timeTamp == 0 {
			this.resqRingWindow[location].timeTamp = addTime
		}
		this.resqRingWindow[location].val++
		return
	}
}

func (this *SlidingWindow) GetResqCnt() uint32 {
	this.lock.RLock()
	defer this.lock.RUnlock()
	bucket := time.Now().Local().Unix() - 10
	var reqTotalCnt uint32 = 0
	for _, count := range this.resqRingWindow {
		if count.timeTamp <= bucket {
			continue
		}
		reqTotalCnt += count.val
	}
	return reqTotalCnt
}

func (this *SlidingWindow) Clear() {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.startTime = time.Now().Local().Unix()

	for idx, count := range this.resqRingWindow {
		count.val = 0
		count.timeTamp = this.startTime + int64(idx)
	}
	this.pStart = 0
	this.pEnd = int(this.windowSize) - 1
}
