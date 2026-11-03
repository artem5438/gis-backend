package db

import (
	"context"
	"log"
	"os"

	"gis-backend/config"

	"github.com/redis/go-redis/v9"
)

var Redis *redis.Client

func ConnectRedis() {
	Redis = redis.NewClient(&redis.Options{Addr: config.RedisAddr()})
	if err := Redis.Ping(context.Background()).Err(); err != nil {
		log.Printf("redis connection failed: %v", err)
		os.Exit(1)
	}
	log.Print("redis connected")
}
