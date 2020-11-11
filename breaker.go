package breaker

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/context"
)

var (
	/*熔断器已经打开*/
	BREAKER_OPEN_ERROR error = errors.New("breaker_open")
	/*熔断器重试中请求被限流*/
	LIMIT_ERROR error = errors.New("limit")
	/*熔断器允许重试并计数*/
	LIMIT_PASS_ERROR error = errors.New("limit_pass")
	/*熔断器熔断超时需要状态流转*/
	OPEN_TO_HALF_ERROR error = errors.New("OPEN_TO_HALF")
)

var (
	/*方法为空*/
	FUNC_NIL_ERROR error = errors.New("runFunc nil")
	/*策略名为空*/
	NAME_NIL_ERROR error = errors.New("name nil")
)

/*
熔断器
*/
type breaker struct {
	/*策略名*/
	name string
	/*熔断器关闭时错误占比*/
	errorPercentThreshold int
	/*熔断限流期间错误占比*/
	breakerErrorPercentThreshold int
	/*统计收集的间隔*/
	interval int64
	/*熔断休眠时间*/
	sleepWindow int64
	/*熔断器状态*/
	state int32
	/*周期管理*/
	cycleTime int64
	/*统计器*/
	reqBc  *SlidingWindow
	reqCc  *SlidingWindow
	failBc *SlidingWindow
	failCc *SlidingWindow
	/*半开启时触发状态转换需要的请求次数*/
	breakerTestMax int
	/*熔断时令牌桶*/
	lpm *limitPoolManager
}

/*
熔断器管理器
*/
type breakerManager struct {
	/*读写锁*/
	mutex *sync.RWMutex
	/*Breaker集合*/
	manager map[string]*breaker
}

/*定义全局熔断器管理器*/
var bm *breakerManager

/*构造全局熔断器管理器*/
func init() {
	bm = new(breakerManager)
	bm.manager = make(map[string]*breaker)
	bm.mutex = &sync.RWMutex{}
}

/*
执行函数
*/
type runFunc func() error

/*
回调函数
*/
type fallbackFunc func(error)

/*
方法创建一个熔断器
*/
func newBreaker(b *breakSettingInfo) *breaker {
	lpm := NewLimitPoolManager(b.BreakerTestMax)
	reqBc := NewSlidingWindow(SlidingWindowSetting{CycleTime: b.Interval})
	reqCc := NewSlidingWindow(SlidingWindowSetting{CycleTime: b.Interval})
	failBc := NewSlidingWindow(SlidingWindowSetting{CycleTime: b.Interval})
	failCc := NewSlidingWindow(SlidingWindowSetting{CycleTime: b.Interval})
	return &breaker{
		name:                         b.Name,
		breakerErrorPercentThreshold: b.BreakerErrorPercentThreshold,
		errorPercentThreshold:        b.ErrorPercentThreshold,
		state:                        STATE_CLOSED,
		interval:                     b.Interval,
		cycleTime:                    time.Now().Local().Unix() + b.Interval,
		sleepWindow:                  b.SleepWindow,
		reqBc:                        reqBc,
		reqCc:                        reqCc,
		failBc:                       failBc,
		failCc:                       failCc,
		breakerTestMax:               b.BreakerTestMax,
		lpm:                          lpm,
	}
}

/*
方法从管理器获得熔断器
*/
func getBreakerManager(name string) (*breaker, error) {
	bm.mutex.RLock()
	pBreaker, ok := bm.manager[name]
	if !ok {
		bm.mutex.RUnlock()
		bm.mutex.Lock()
		defer bm.mutex.Unlock()
		if pBreaker, ok := bm.manager[name]; ok {
			return pBreaker, nil
		}
		pbreak, err := NewBreakSettingInfo().SetName(name).AddBreakSetting()
		if err != nil {
			return nil, err
		}
		bm.manager[name] = pbreak
		return pbreak, nil
	} else {
		defer bm.mutex.RUnlock()
		return pBreaker, nil
	}
}

/*
状态转换
*/
func (this *breaker) updateState(oldStatus, state int32) bool {
	return atomic.CompareAndSwapInt32(&this.state, oldStatus, state)
}

func (this *breaker) getFailPercentThreshold() (bool, int) {
	reqChan := make(chan uint32)
	failChan := make(chan uint32)
	var reqCnt uint32 = 0
	var failCnt uint32 = 0
	times := 0
	go func() { reqChan <- this.reqCc.GetResqCnt() }()
	go func() { failChan <- this.failCc.GetResqCnt() }()

Loop:
	for {
		select {
		case reqCnt = <-reqChan:
			times++
			if times >= 2 {
				break Loop
			}
		case failCnt = <-failChan:
			times++
			if times >= 2 {
				break Loop
			}
		}
	}
	if reqCnt < 10 {
		return false, 0
	}
	return true, int(float32(failCnt)/float32(reqCnt)*100 + 0.5)
}

func (this *breaker) getBreakFailPercentThreshold() (bool, int) {
	reqChan := make(chan uint32)
	failChan := make(chan uint32)
	var reqCnt uint32 = 0
	var failCnt uint32 = 0
	times := 0
	go func() { reqChan <- this.reqBc.GetResqCnt() }()
	go func() { failChan <- this.failBc.GetResqCnt() }()

Loop:
	for {
		select {
		case reqCnt = <-reqChan:
			times++
			if times >= 2 {
				break Loop
			}
		case failCnt = <-failChan:
			times++
			if times >= 2 {
				break Loop
			}
		}
	}
	if reqCnt < uint32(this.breakerTestMax) {
		return false, 0
	}
	return true, int(float32(failCnt)/float32(reqCnt)*100 + 0.5)
}

/*
方法失败处理
*/
func (this *breaker) fail() {
	switch this.state {
	case STATE_CLOSED:
		var wg sync.WaitGroup
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			this.reqCc.Add()
		}(&wg)
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			this.failCc.Add()
		}(&wg)
		wg.Wait()

		ok, errorPercentThreshold := this.getFailPercentThreshold()
		if this.state == STATE_CLOSED && ok && errorPercentThreshold >= this.errorPercentThreshold {
			this.cycleTime = time.Now().Local().Unix() + this.sleepWindow
			//STATE_CLOSED---->STATE_OPEN
			this.updateState(STATE_CLOSED, STATE_OPEN)
		}

	case STATE_HALFOPEN:
		var wg sync.WaitGroup
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			this.reqBc.Add()
		}(&wg)
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			this.failBc.Add()
		}(&wg)
		wg.Wait()

		ok, errorPercentThreshold := this.getBreakFailPercentThreshold()
		if this.state == STATE_HALFOPEN && ok {
			defer this.lpm.ReturnAll()
			defer this.failBc.Clear()
			defer this.reqBc.Clear()

			if errorPercentThreshold >= this.breakerErrorPercentThreshold {
				this.cycleTime = time.Now().Local().Unix() + this.sleepWindow
				//STATE_HALFOPEN---->STATE_OPEN
				if this.updateState(STATE_HALFOPEN, STATE_OPEN) {

				}
			} else {
				//STATE_HALFOPEN---->STATE_CLOSED
				this.updateState(STATE_HALFOPEN, STATE_CLOSED)
			}
		}
	}
}

/*
方法成功处理
*/
func (this *breaker) success() {
	switch this.state {
	case STATE_CLOSED:
		this.reqCc.Add()
	case STATE_HALFOPEN:
		this.reqBc.Add()
		ok, errorPercentThreshold := this.getBreakFailPercentThreshold()
		if this.state == STATE_HALFOPEN && ok {
			defer this.lpm.ReturnAll()
			defer this.failBc.Clear()
			defer this.reqBc.Clear()
			if errorPercentThreshold < this.breakerErrorPercentThreshold {
				//STATE_HALFOPEN---->STATE_CLOSED
				this.updateState(STATE_HALFOPEN, STATE_CLOSED)
			} else {
				this.cycleTime = time.Now().Local().Unix() + this.sleepWindow
				//STATE_HALFOPEN---->STATE_OPEN
				this.updateState(STATE_HALFOPEN, STATE_OPEN)
			}
		}
	}
}

/*
包装外部回调函数
*/
func (this *breaker) safelCalllback(fallback fallbackFunc, err error) {
	if fallback == nil {
		return
	}
	fallback(err)
}

func safelCalllback(fallback fallbackFunc, err error) {
	if fallback == nil {
		return
	}
	fallback(err)
}

/*
执行方法前的处理
*/
func (this *breaker) beforeDo(ctx context.Context, name string) error {
	switch this.state {
	case STATE_HALFOPEN:
		return LIMIT_ERROR
	case STATE_OPEN:
		if this.cycleTime < time.Now().Local().Unix() {
			return OPEN_TO_HALF_ERROR
		}
		return BREAKER_OPEN_ERROR
	}
	return nil
}

/*
执行方法后的处理
*/
func (this *breaker) afterDo(ctx context.Context, run runFunc, fallback fallbackFunc, err error) error {
	switch err {
	/*熔断时*/
	case BREAKER_OPEN_ERROR:
		this.safelCalllback(fallback, BREAKER_OPEN_ERROR)
		return BREAKER_OPEN_ERROR
	/*熔断转移到半开启*/
	case OPEN_TO_HALF_ERROR:
		/*取令牌*/
		if !this.lpm.GetTicket() {
			this.safelCalllback(fallback, BREAKER_OPEN_ERROR)
			return BREAKER_OPEN_ERROR
		}
		//状态转移到半开启
		this.updateState(STATE_OPEN, STATE_HALFOPEN)
		/*执行方法*/
		runErr := run()
		if runErr != nil {
			this.fail()
			this.safelCalllback(fallback, runErr)
			return runErr
		}
		this.success()
		return nil
	/*熔断限流开始，半开启状态转移到开启状态或者关闭状态*/
	case LIMIT_ERROR:
		//如果令牌用完
		if !this.lpm.GetTicket() {
			this.safelCalllback(fallback, BREAKER_OPEN_ERROR)
			return BREAKER_OPEN_ERROR
		}
		runErr := run()
		if runErr != nil {
			this.fail()
			this.safelCalllback(fallback, runErr)
			return runErr
		}
		this.success()
		return nil
	default:
		if err != nil {
			this.fail()
			this.safelCalllback(fallback, err)
			return err
		}
		this.success()
		return nil
	}
}

/*
Do方法结合熔断策略执行run函数
其中参数包括:上下文ctx,策略名name,将要执行方法run,以及回调函数fallback.
其中ctx,name,run必传
*/
func Do(ctx context.Context, name string, run runFunc, fallback fallbackFunc) error {
	if run == nil {
		return FUNC_NIL_ERROR
	}
	if name == "" {
		return NAME_NIL_ERROR
	}
	//获得熔断器
	pBreaker, err := getBreakerManager(name)
	if err != nil {
		fallback(err)
		return err
	}
	//判断当前是否可以请求
	beforeDoErr := pBreaker.beforeDo(ctx, name)
	if beforeDoErr != nil {
		callBackErr := pBreaker.afterDo(ctx, run, fallback, beforeDoErr)
		return callBackErr
	}
	runErr := run()
	//执行后的处理
	return pBreaker.afterDo(ctx, run, fallback, runErr)
}
