package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"gitlab.sftcwl.com/golang-lib-inner/breaker"
)

func main() {

	// var wg0 sync.WaitGroup
	// for i := 0; i < 10; i++ {
	// 	wg0.Add(1)
	// 	go func(i int, wg0 *sync.WaitGroup) {
	// 		defer wg0.Done()
	// 		err := breaker.Do(context.Background(), "test", func() error {
	// 			url := "https://www.baidu.com/"
	// 			_, err := http.Get(url)
	// 			if err != nil {
	// 				return err
	// 			}
	// 			return nil
	// 		}, nil)
	// 		if err != nil {
	// 			fmt.Println("并发0 error", err, i)
	// 		} else {
	// 			fmt.Println("并发0 sucess", i)
	// 		}
	// 	}(i, &wg0)
	// }
	// wg0.Wait()

	//模拟并发时某台机器有问题
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int, wg *sync.WaitGroup) {
			defer wg.Done()
			breaker.Do(context.Background(), "test", func() error {
				url := "https://www.baidu.com/"
				if i%2 == 0 {
					url = "https://www.baidu.co"
				}
				_, err := http.Get(url)
				if err != nil {
					return err
				}
				return nil
			}, func(err error) {
				fmt.Println("并发 callback", err)
			})
		}(i, &wg)
	}
	wg.Wait()

	time.Sleep(time.Millisecond * 2000)
	/*查看熔断是否成功*/
	var wg2 sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg2.Add(1)
		go func(i int, wg2 *sync.WaitGroup) {
			defer wg2.Done()
			err := breaker.Do(context.Background(), "test", func() error {
				url := "https://www.baidu.com/"
				resp, err := http.Get(url)
				if err != nil {
					return err
				}

				defer resp.Body.Close()
				if _, err := ioutil.ReadAll(resp.Body); err != nil {
					return err
				}
				return nil
			}, func(err error) {
				fmt.Println("并发2 callback", err)
			})
			if err != nil {
				fmt.Println("并发2 error", err, i)
			}
		}(i, &wg2)
	}
	wg2.Wait()

	// /*睡眠到休眠结束*/
	time.Sleep(time.Second * 28)
	/*查看半开启是否成功*/
	var wg3 sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg3.Add(1)
		go func(i int, wg3 *sync.WaitGroup) {
			defer wg3.Done()
			breaker.Do(context.Background(), "test", func() error {
				url := "https://www.baidu.com/"
				resp, err := http.Get(url)
				if err != nil {
					return err
				}

				defer resp.Body.Close()
				if _, err := ioutil.ReadAll(resp.Body); err != nil {
					return err
				}
				return nil
			}, func(err error) {
				if err != nil {
					fmt.Println("并发3 callback", err)
				} else {
					fmt.Println("并发3 ok")
				}
			})
		}(i, &wg3)
	}
	wg3.Wait()

	/*查看恢复是否成功*/
	var wg4 sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg4.Add(1)
		go func(i int, wg4 *sync.WaitGroup) {
			defer wg4.Done()
			breaker.Do(context.Background(), "test", func() error {
				url := "https://www.baidu.com/"
				_, err := http.Get(url)
				if err != nil {
					return err
				}
				return nil
			}, func(err error) {
				if err != nil {
					fmt.Println("并发4 callback", err)
				} else {
					fmt.Println("并发4 ok")
				}
			})
		}(i, &wg4)
	}
	wg4.Wait()

	time.Sleep(time.Second * 2)

	/*睡眠到下一次统计周期*/
	time.Sleep(time.Second * 60)
	/*再次模拟正常流量*/
	var wg5 sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg5.Add(1)
		go func(i int, wg5 *sync.WaitGroup) {
			defer wg5.Done()
			breaker.Do(context.Background(), "test", func() error {
				url := "https://www.baidu.com/"
				resp, err := http.Get(url)
				if err != nil {
					return err
				}

				defer resp.Body.Close()
				if _, err := ioutil.ReadAll(resp.Body); err != nil {
					return err
				}
				return nil
			}, func(err error) {
				if err != nil {
					fmt.Println("并发5 callback", err)
				} else {
					fmt.Println("并发5 ok")
				}
			})
		}(i, &wg5)
	}
	wg5.Wait()

	/*再次模拟错误流量*/
	var wg6 sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg6.Add(1)
		go func(i int, wg6 *sync.WaitGroup) {
			defer wg6.Done()
			breaker.Do(context.Background(), "test", func() error {
				url := "https://www.baidu.co/"
				resp, err := http.Get(url)
				if err != nil {
					return err
				}

				defer resp.Body.Close()
				if _, err := ioutil.ReadAll(resp.Body); err != nil {
					return err
				}
				return nil
			}, func(err error) {
				if err != nil {
					fmt.Println("并发6 callback", err)
				} else {
					fmt.Println("并发6 ok")
				}
			})
		}(i, &wg6)
	}
	wg6.Wait()

	/*再次查看熔断是否成功*/
	var wg7 sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg7.Add(1)
		go func(i int, wg7 *sync.WaitGroup) {
			defer wg7.Done()
			breaker.Do(context.Background(), "test", func() error {
				url := "https://www.baidu.com/"
				resp, err := http.Get(url)
				if err != nil {
					return err
				}

				defer resp.Body.Close()
				if _, err := ioutil.ReadAll(resp.Body); err != nil {
					return err
				}
				return nil
			}, func(err error) {
				if err != nil {
					fmt.Println("并发7 callback", err)
				} else {
					fmt.Println("并发7 ok")
				}
			})
		}(i, &wg7)
	}
	wg7.Wait()
}
