package client

import (
    pb "github.com/ouqiang/gocron/modules/rpc/proto"
    "golang.org/x/net/context"
    "fmt"
    "time"
    "errors"
    "github.com/ouqiang/gocron/modules/rpc/grpcpool"
    "google.golang.org/grpc/codes"
    "google.golang.org/grpc"
    "github.com/ouqiang/gocron/modules/logger"
)

var (
    errUnavailable = errors.New("无法连接远程服务器")
)

func ExecWithRetry(ip string, port int, certFile string, token string, taskReq *pb.TaskRequest) (string, error)  {
    tryTimes := 15
    i := 0
    for i < tryTimes {
        output, err := Exec(ip, port, certFile, token, taskReq)
        if err != errUnavailable {
            return output, err
        }
        i++
        time.Sleep(2 * time.Second)
    }

    return "", errUnavailable
}

func Exec(ip string, port int, certFile string, token string, taskReq *pb.TaskRequest) (string, error)  {
    defer func() {
       if err := recover(); err != nil {
           logger.Error("panic#rpc/client.go:Exec#", err)
       }
    } ()
    addr := fmt.Sprintf("%s:%d", ip, port)
    conn, err := grpcpool.Pool.Get(addr, certFile, token)
    if err != nil {
        return "", err
    }
    isConnClosed := false
    defer func() {
        if !isConnClosed {
            grpcpool.Pool.Put(addr, conn)
        }
    }()
    c := pb.NewTaskClient(conn)
    if taskReq.Timeout <= 0 || taskReq.Timeout > 86400 {
        taskReq.Timeout = 86400
    }
    timeout := time.Duration(taskReq.Timeout) * time.Second
    ctx, cancel := context.WithTimeout(context.Background(), timeout)
    defer cancel()
    resp, err := c.Run(ctx, taskReq)
    if err != nil {
       return parseGRPCError(err, conn, &isConnClosed)
    }

    if resp.Error == "" {
        return resp.Output, nil
    }

    return resp.Output, errors.New(resp.Error)
}

func parseGRPCError(err error, conn *grpc.ClientConn, connClosed *bool) (string, error) {
    switch grpc.Code(err) {
        case codes.Unavailable, codes.Internal:
            conn.Close()
            *connClosed = true
            return "", errUnavailable
        case codes.DeadlineExceeded:
            return "", errors.New("执行超时, 强制结束")
    }
    return "", err
}