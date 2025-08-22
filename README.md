# 氙-Xe

[![golang](https://img.shields.io/badge/Language-Go-green.svg?style=flat)](https://golang.org)
[![pkg.go.dev](https://img.shields.io/badge/dev-reference-007d9c?logo=go&logoColor=white&style=flat)](https://pkg.go.dev/github.com/noble-gase/xe)
[![MIT](http://img.shields.io/badge/license-MIT-brightgreen.svg)](http://opensource.org/licenses/MIT)

[氙-Xe] Go协程并发复用，降低CPU和内存负载

## 特点

1. 实现简单
2. 性能优异
3. 采用「生产-消费」模式
4. 任务支持 `context`
5. 支持任务缓存队列，缓存达上限会阻塞等待
6. 基于官方版本实现的 `errgroup`，支持协程数量控制
7. 简单实用的单层时间轮（支持一次性和多次重试任务）

## 安装

```shell
go get -u github.com/noble-gase/xe
```

## 流程图

![flowchart.jpg](example/flowchart.jpg)

## 效果

```shell
goos: darwin
goarch: arm64
cpu: Apple M4
```

### 场景-1

```go
func main() {
    ctx := context.Background()

    pool := woker.New(5000)
    for i := 0; i < 100000000; i++ {
        i := i
        pool.Go(ctx, func(ctx context.Context) {
            time.Sleep(time.Second)
            fmt.Println("Index:", i)
        })
    }

    <-ctx.Done()
}
```

#### cpu

![cpu-1.png](example/cpu-1.png)

#### mem

![mem-1.png](example/mem-1.png)

### 场景-2

```go
func main() {
    ctx := context.Background()

    pool := woker.New(5000)
    for i := 0; i < 100; i++ {
        i := i
        pool.Go(ctx, func(ctx context.Context) {
            for j := 0; j < 1000000; j++ {
                j := j
                pool.Go(ctx, func(ctx context.Context) {
                    time.Sleep(time.Second)
                    fmt.Println("Index:", i, "-", j)
                })
            }
        })
    }

    <-ctx.Done()
}
```

#### cpu

![cpu-2.png](example/cpu-2.png)

#### mem

![mem-2.png](example/mem-2.png)
