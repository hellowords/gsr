# session-redis

session store backend for [gorilla/sessions](http://www.gorillatoolkit.org/pkg/sessions) - [src](https://github.com/gorilla/sessions).

## Requirements

Base on [go-redis](https://github.com/go-redis/redis)

## Installation

```go
go get -u github.com/hellowords/go-session-redis
```

## Example

```go
client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		PoolSize: 10,
	})
store, err := NewRedisStoreWithDB(client, []byte("new-key"))
session ,err = store.Get(req,"session-key")
session.Values["hello"] = "bar"
```

## Reference

[github.com/boj/redistore](https://github.com/boj/redistore)