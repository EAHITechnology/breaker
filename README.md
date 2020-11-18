# breaker
  microservice fuse developed based on golang
  基于golang的微服务熔断器

  Project Description:Microservice Fuse
  项目简介：微服务熔断器
  
### 引入
```
	//预先设置熔断策略,也可不添加，不添加采用默认配置
	breaker.NewBreakSettingInfo().SetName("test").SetErrorPercentThreshold(50).SetSleepWindow(10).....

	//回调函数，如果执行失败将执行降级回调方法，没有降级函数默认此项填nil
	fallbackFunc:=func(err error){
		//todo 自定义降级策略
	}

	//执行函数
	runFunc:=func(err error)error{
		//todo 自定义将要执行方法
	}

	//熔断入口
    	err := breaker.Do(context.Background(), "test", runFunc, fallbackFunc)
	if err != nil {
		fmt.Println("error", err)
	}
```
### 返回
    除了执行方法返回的错误,显示 BREAKER_OPEN_ERROR 错误(错误字符串为 breaker_open )则正处于熔断当中，次情况可能对应两种状态中的一种:
	1、熔断中
	2、在半开启时多余请求被限流

### 配置
```
	Name                         string // 策略名
	Interval                     int64  // 采样时间间隔
	SleepWindow                  int64  // 熔断休眠时间
	BreakerTestMax               int    // 半开启后重试次数
	ErrorPercentThreshold        int    // 熔断器关闭时错误占比
	BreakerErrorPercentThreshold int.   // 熔断器半开启重试时错误占比
```
### 测试用例
    ./test/test.go
