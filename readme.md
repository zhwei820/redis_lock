```go

package redis_lock_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/zhwei820/gconv/grand"

	"github.com/zhwei820/redis_lock"
	rc "github.com/zhwei820/redisclient"
)

type RedisTestSuite struct {
	suite.Suite
	cli  rc.RedisCli
	lock *redis_lock.RedLock
}

var ctx = context.Background()

func (r *RedisTestSuite) SetupTest() {
	_ = r.cli.FlushDB(ctx)
}

func TestRedisTestSuite(t *testing.T) {
	cfg := &rc.Config{Host: "127.0.0.1:6379", Password: "123456", DbNumber: 15, DialTimeout: 1, MaxIdle: 2}
	cli := rc.NewRedisClientV2(cfg)
	lock := redis_lock.NewRedLock(cli)
	suite.Run(t, &RedisTestSuite{cli: cli, lock: lock})
}

func (r *RedisTestSuite) Test_Lock() {
	ctx := context.Background()
	lockKey := "lock_key"
	secret := grand.Letters(10)
	ok, err := r.lock.Lock(ctx, lockKey, secret)
	if err != nil {
		panic(err)
	}
	defer func() {
		ok, err := r.lock.Release(ctx, lockKey, secret)
		assert.True(r.T(), ok)
		assert.Nil(r.T(), err)
	}()
	fmt.Println("ok", ok)

	ok, err = r.lock.Lock(ctx, lockKey, secret) // same secret can get lock again
	if err != nil {
		panic(err)
	}
	fmt.Println("ok", ok)

	ok, err = r.lock.Lock(ctx, lockKey, grand.Letters(10))
	fmt.Println("ok", ok)
	assert.NotNil(r.T(), err)

	time.Sleep(10 * time.Second)

	fmt.Println("===> after 10 seconds, should be renewed")
	ok, err = r.lock.Lock(ctx, lockKey, grand.Letters(10))
	fmt.Println("ok", ok)
	assert.NotNil(r.T(), err)
}



```