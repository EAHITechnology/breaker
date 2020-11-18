package breaker

import (
	"errors"
)

var (
	DefaultInterval                     int64 = 60
	DefaultSleepWindow                  int64 = 65
	DefaultBreakerTestMax                     = 20
	DefaultErrorPercentThreshold              = 50
	DefaultBreakerErrorPercentThreshold       = 50
)

type breakSettingInfo struct {
	Name                         string
	Interval                     int64
	SleepWindow                  int64
	BreakerTestMax               int
	ErrorPercentThreshold        int
	BreakerErrorPercentThreshold int
}

/*
新建熔断器配置
*/
func NewBreakSettingInfo() *breakSettingInfo {
	return &breakSettingInfo{}
}

/*
设置策略名
*/
func (this *breakSettingInfo) SetName(name string) *breakSettingInfo {
	this.Name = name
	return this
}

/*
设置方法错误比
*/
func (this *breakSettingInfo) SetErrorPercentThreshold(errorPercentThreshold int) *breakSettingInfo {
	this.ErrorPercentThreshold = errorPercentThreshold
	return this
}

/*
设置熔断休眠时间
*/
func (this *breakSettingInfo) SetSleepWindow(sleepWindow int64) *breakSettingInfo {
	this.SleepWindow = sleepWindow
	return this
}

/*
设置采样周期
*/
func (this *breakSettingInfo) SetInterval(interval int64) *breakSettingInfo {
	this.Interval = interval
	return this
}

/*
设置熔断最大测试次数
*/
func (this *breakSettingInfo) SetBreakerTestMax(breakerTestMax int) *breakSettingInfo {
	this.BreakerTestMax = breakerTestMax
	return this
}

/*
添加配置方法，最后将期望配置添加到熔断器管理器里，如果策略名为空字符串则报错
*/
func (this *breakSettingInfo) AddBreakSetting() (*breaker, error) {
	if this.Name == "" {
		return nil, errors.New("name nil")
	}
	if this.BreakerErrorPercentThreshold <= 0 {
		this.BreakerErrorPercentThreshold = DefaultBreakerErrorPercentThreshold
	}
	if this.ErrorPercentThreshold <= 0 {
		this.ErrorPercentThreshold = DefaultErrorPercentThreshold
	}
	if this.BreakerTestMax < DefaultBreakerTestMax {
		this.BreakerTestMax = DefaultBreakerTestMax
	}
	if this.Interval < DefaultInterval {
		this.Interval = DefaultInterval
	}
	if this.SleepWindow < DefaultInterval {
		this.SleepWindow = DefaultSleepWindow
	}
	if this.Interval%int64(cellCnt) != 0 {
		return nil, errors.New("interval must 20`s multiple")
	}
	pbreaker := newBreaker(this)
	return pbreaker, nil
}
