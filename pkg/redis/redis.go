package redis

import (
    "context"
    "github.com/redis/go-redis/v9"
    "time"
)

type Client struct {
    *redis.Client
}

func NewClient(addr string) (*Client, error) {
    rdb := redis.NewClient(&redis.Options{
        Addr:        addr,
        DialTimeout: 5 * time.Second,
    })
    if err := rdb.Ping(context.Background()).Err(); err != nil {
        return nil, err
    }
    return &Client{Client: rdb}, nil
}
