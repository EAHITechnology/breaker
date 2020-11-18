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
	/*熔断休眠时间*/
	sleepWindow int64
	/*周期管理*/
	cycleTime int64
	/*统计器*/
	counter *SlidingWindow
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
	counter := NewSlidingWindow(SlidingWindowSetting{CycleTime: b.Interval,
		ErrorPercent:      b.ErrorPercentThreshold,
		BreakErrorPercent: b.BreakerErrorPercentThreshold,
		BreakCnt:          b.BreakerTestMax,
	})
	return &breaker{
		name:        b.Name,
		cycleTime:   time.Now().Local().Unix() + b.SleepWindow,
		sleepWindow: b.SleepWindow,
		counter:     counter,
		lpm:         lpm,
	}
}

/*
方法从管理器获得熔断器
*/
func getBreakerManager(name string) (*breaker, error) {
	if name == "" {
		return nil, errors.New("no name")
	}
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
方法失败处理
*/
func (this *breaker) fail() {
	state := this.counter.GetStatus()
	switch state {
	case STATE_CLOSED:
		atomic.StoreInt64(&this.cycleTime, time.Now().Local().Unix()+this.sleepWindow)
		this.counter.Add(false)
	case STATE_OPEN:
		if time.Now().Local().Unix() > atomic.LoadInt64(&this.cycleTime) {
			if this.counter.AddBreak(false) {
				defer this.lpm.ReturnAll()
				atomic.StoreInt64(&this.cycleTime, time.Now().Local().Unix()+this.sleepWindow)
			}
		}
	}
}

/*
方法成功处理
*/
func (this *breaker) success() {
	state := this.counter.GetStatus()
	switch state {
	case STATE_CLOSED:
		atomic.StoreInt64(&this.cycleTime, time.Now().Local().Unix()+this.sleepWindow)
		this.counter.Add(true)
	case STATE_OPEN:
		if time.Now().Local().Unix() > atomic.LoadInt64(&this.cycleTime) {
			if this.counter.AddBreak(true) {
				defer this.lpm.ReturnAll()
				atomic.StoreInt64(&this.cycleTime, time.Now().Local().Unix()+this.sleepWindow)
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
	switch this.counter.GetStatus() {
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
		return nil
	/*熔断转移到半开启*/
	case OPEN_TO_HALF_ERROR:
		/*取令牌*/
		if !this.lpm.GetTicket() {
			this.safelCalllback(fallback, BREAKER_OPEN_ERROR)
			return nil
		}
		/*执行方法*/
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

其中参数包括:上下文ctx,策略名name,将要执行方法run,以及回调函数fallback.其中ctx,name,run必传

run函数的错误会直接同步返回，回调函数fallback接收除了run错误以外还会接收熔断时错误，调用方如果需要降级可在fallback中自己判断
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
		//如果有错误直接交给afterDo处理
		callBackErr := pBreaker.afterDo(ctx, run, fallback, beforeDoErr)
		return callBackErr
	}
	runErr := run()
	//执行后的处理
	return pBreaker.afterDo(ctx, run, fallback, runErr)
}
