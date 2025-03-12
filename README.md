# 氙-Xe

[![golang](https://img.shields.io/badge/Language-Go-green.svg?style=flat)](https://golang.org)
[![pkg.go.dev](https://img.shields.io/badge/dev-reference-007d9c?logo=go&logoColor=white&style=flat)](https://pkg.go.dev/github.com/noble-gase/xe)
[![Apache 2.0 license](http://img.shields.io/badge/license-Apache%202.0-brightgreen.svg)](http://opensource.org/licenses/apache2.0)

[氙-Xe] Go协程并发复用，降低CPU和内存负载

## 特点

1. 实现简单
2. 性能优异
3. 采用「生产-消费」模式
4. 任务支持 `context`
5. 任务队列支持缓冲大小设置
6. 异步模式下，任务缓存到全局链表

## 安装

```shell
go get -u github.com/noble-gase/xe
```

## 流程图

![flowchart.jpg](example/flowchart.jpg)

## 效果

```shell
goos: darwin
goarch: amd64
cpu: Intel(R) Core(TM) i5-1038NG7 CPU @ 2.00GHz
```

### 场景-1

#### 👉 xe

```go
func main() {
    ctx := context.Background()

    pool := woker.NewPool(5000)
    for i := 0; i < 100000000; i++ {
        i := i
        pool.Sync(ctx, func(ctx context.Context) {
            time.Sleep(time.Second)
            fmt.Println("Index:", i)
        })
    }

    <-ctx.Done()
}
```

##### cpu

![nightfall_cpu_1.png](example/nightfall_cpu_1.png)

##### mem

![nightfall_mem_1.png](example/nightfall_mem_1.png)

#### 👉 ants

```go
func main() {
    ctx := context.Background()

    pool, _ := ants.NewPool(5000)
    for i := 0; i < 100000000; i++ {
        i := i
        pool.Submit(func() {
            time.Sleep(time.Second)
            fmt.Println("Index:", i)
        })
    }

    <-ctx.Done()
}
```

##### cpu

![ants_cpu_1.png](example/ants_cpu_1.png)

##### mem

![ants_mem_1.png](example/ants_mem_1.png)

### 场景-2

#### 👉 xe

```go
func main() {
    ctx := context.Background()

    pool := woker.NewPool(5000)
    for i := 0; i < 100; i++ {
        i := i
        pool.Sync(ctx, func(ctx context.Context) {
            for j := 0; j < 1000000; j++ {
                j := j
                pool.Sync(ctx, func(ctx context.Context) {
                    time.Sleep(time.Second)
                    fmt.Println("Index:", i, "-", j)
                })
            }
        })
    }

    <-ctx.Done()
}
```

##### cpu

![nightfall_cpu_2.png](example/nightfall_cpu_2.png)

##### mem

![nightfall_mem_2.png](example/nightfall_mem_2.png)

#### 👉 ants

```go
func main() {
    ctx := context.Background()

    pool, _ := ants.NewPool(5000)
    for i := 0; i < 100; i++ {
        i := i
        pool.Submit(func() {
            for j := 0; j < 1000000; j++ {
                j := j
                pool.Submit(func() {
                    time.Sleep(time.Second)
                    fmt.Println("Index:", i, "-", j)
                })
            }
        })
    }

    <-ctx.Done()
}
```

##### cpu

![ants_cpu_2.png](example/ants_cpu_2.png)

##### mem

![ants_mem_2.png](example/ants_mem_2.png)
