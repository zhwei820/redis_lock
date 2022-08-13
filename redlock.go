package redis_lock

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zhwei820/log"
	"github.com/zhwei820/redisclient"

	"github.com/garyburd/redigo/redis"
	"github.com/pkg/errors"
)

var ErrLockMismatch = errors.Errorf("key is locked with a different secret")

const lockScriptStr = `
 local v = redis.call("GET", KEYS[1])
if v == false or v == ARGV[1]
then
	return redis.call("SET", KEYS[1], ARGV[1], "EX", ARGV[2]) and 1
else
	return 0
end
`
const renewScriptStr = `
 local v = redis.call("GET", KEYS[1])
if v == ARGV[1]
then
	return redis.call("SET", KEYS[1], ARGV[1], "EX", ARGV[2]) and 1
else
	return 0
end
`

const unlockScriptStr = `
local v = redis.call("GET",KEYS[1])
if v == false then
	return 1
elseif v == ARGV[1] then
	return redis.call("DEL",KEYS[1])
else
	return 0
end
`

type RedLock struct {
	redisClient redisclient.RedisCli
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewRedLock(redisClient redisclient.RedisCli) *RedLock {
	ctx, cancel := context.WithCancel(context.Background())
	obj := &RedLock{redisClient: redisClient,
		ctx:    ctx,
		cancel: cancel,
	}
	rc := obj.redisClient.GetConn(context.Background())
	defer rc.Close()

	fmt.Println("send lockScript", lockScript.Send(rc, "NewRedLock-name", "secret", 1))
	fmt.Println("send unlockScript", unlockScript.Send(rc, "NewRedLock-name", "secret"))
	fmt.Println("send renewScript", renewScript.Send(rc, "NewRedLock-name", "secret"))

	go obj.autoRenew()
	return obj
}

var renewKeys sync.Map

var lockScript = redis.NewScript(1, lockScriptStr)
var renewScript = redis.NewScript(1, renewScriptStr)
var unlockScript = redis.NewScript(1, unlockScriptStr)

// writeLock attempts to grab a redis lock. The error returned is safe to ignore
// if all you care about is whether or not the lock was acquired successfully.
func (c *RedLock) Lock(ctx context.Context, name, secret string) (bool, error) {
	rc := c.redisClient.GetConn(ctx)
	defer rc.Close()

	resp, err := redis.Int(lockScript.Do(rc, name, secret, 10))
	if err != nil {
		return false, err
	}
	if resp == 0 {
		return false, ErrLockMismatch
	}

	renewKeys.Store(name, secret)
	return true, nil
}
func (c *RedLock) renew(name, secret string, ttl uint64) (bool, error) {
	rc := c.redisClient.GetConn(context.Background())
	defer rc.Close()

	resp, err := redis.Int(renewScript.Do(rc, name, secret, int64(ttl)))
	if err != nil {
		return false, err
	}
	if resp == 0 {
		return false, ErrLockMismatch
	}
	return true, nil
}

// writeLock releases the redis lock
func (c *RedLock) Release(ctx context.Context, name, secret string) (bool, error) {
	rc := c.redisClient.GetConn(ctx)
	defer rc.Close()
	renewKeys.Delete(name)

	resp, err := redis.Int(unlockScript.Do(rc, name, secret))
	if err != nil {
		return false, err
	}
	if resp == 0 {
		return false, ErrLockMismatch
	}
	return true, nil
}

func (c *RedLock) autoRenew() {
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case <-ticker.C:
			renewKeys.Range(func(key, value interface{}) bool {
				keyStr, _ := key.(string)
				valueStr, _ := value.(string)
				ok, err := c.renew(keyStr, valueStr, 5)
				if err != nil {
					log.WarnZ(context.Background(), "renew lock failed") // ignore renew fail, key has been deleted
				}
				return ok
			})
		case <-c.ctx.Done():
			fmt.Println("closed")
			return
		}
	}
}

func (c *RedLock) Close() {
	c.cancel()
	renewKeys.Range(func(key, value interface{}) bool {
		keyStr, _ := key.(string)
		valueStr, _ := value.(string)
		ok, err := c.Release(context.Background(), keyStr, valueStr)
		if err != nil {
			log.ErrorZ(context.Background(), "renew release failed")
		}
		return ok
	})
}
