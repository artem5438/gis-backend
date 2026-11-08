package parcels

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"gis-backend/auth"
	"gis-backend/db"
	"gis-backend/districts"
	"gis-backend/models"
	"gis-backend/nspd"

	"github.com/redis/go-redis/v9"
)

const (
	queueKey   = "catalog:queue"
	stateKey   = "catalog:state"
	messageKey = "catalog:message"
	freshData  = "Свежие данные не пришли. Сохранённые участки остаются на карте."
)

var (
	walkMu   sync.Mutex
	walking  bool
	walkWake = make(chan struct{}, 1)
)

type walkStatus struct {
	State   string `json:"state"`
	Saved   int64  `json:"saved"`
	Left    int64  `json:"left"`
	Message string `json:"message"`
}

// StartWalker waits until a moderator starts the catalog walk.
// A server restart does not put new rectangles into the queue.
func StartWalker() {
	go func() {
		for range walkWake {
			drain()
		}
	}()
}

// WalkStatus tells how many parcels are saved and how many rectangles remain.
func WalkStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := moderator(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, status())
}

// StartWalk puts the district rectangles into Redis and wakes the worker.
// A second press while the walk is running does not add the rectangles again.
// A press after the walk has stopped starts again from the current contours.
func StartWalk(w http.ResponseWriter, r *http.Request) {
	if _, ok := moderator(w, r); !ok {
		return
	}
	ctx := context.Background()
	if busy() {
		writeJSON(w, http.StatusOK, status())
		return
	}
	if err := enqueueDistricts(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось поставить обход")
		return
	}
	if err := db.Redis.Set(ctx, stateKey, "running", 0).Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось поставить обход")
		return
	}
	select {
	case walkWake <- struct{}{}:
	default:
	}
	writeJSON(w, http.StatusOK, status())
}

func enqueueDistricts(ctx context.Context) error {
	if err := db.Redis.Del(ctx, queueKey, messageKey).Err(); err != nil {
		return err
	}
	tiles := districts.Tiles()
	values := make([]any, 0, len(tiles))
	for _, tile := range tiles {
		encoded, err := json.Marshal(tile)
		if err != nil {
			return err
		}
		values = append(values, string(encoded))
	}
	return db.Redis.RPush(ctx, queueKey, values...).Err()
}

func drain() {
	walkMu.Lock()
	if walking {
		walkMu.Unlock()
		return
	}
	walking = true
	walkMu.Unlock()
	defer func() {
		walkMu.Lock()
		walking = false
		walkMu.Unlock()
	}()

	ctx := context.Background()
	_ = db.Redis.Set(ctx, stateKey, "running", 0).Err()
	misses := 0
	streak := 0
	paused := false
	for {
		raw, err := db.Redis.LPop(ctx, queueKey).Result()
		if errors.Is(err, redis.Nil) {
			break
		}
		if err != nil {
			log.Printf("catalog queue: %v", err)
			misses++
			break
		}
		var tile districts.Tile
		if json.Unmarshal([]byte(raw), &tile) != nil {
			continue
		}
		switch walkTile(ctx, tile) {
		case tileDone:
			streak = 0
		case tileRetry:
			streak++
		case tileMiss:
			misses++
			streak++
		}
		if streak >= 5 {
			paused = true
			log.Print("catalog walk paused: the public map stopped answering")
			break
		}
	}
	message := "Обход закончен."
	if misses > 0 || paused {
		message = freshData
	}
	_ = db.Redis.Set(ctx, messageKey, message, 0).Err()
	_ = db.Redis.Set(ctx, stateKey, "done", 0).Err()
	log.Printf("catalog walk finished, missed rectangles %d", misses)
}

type tileResult int

const (
	tileDone tileResult = iota
	tileRetry
	tileMiss
)

func walkTile(ctx context.Context, tile districts.Tile) tileResult {
	plots, err := nspd.LandPlotsIn(tile.Ring())
	time.Sleep(250 * time.Millisecond)
	if errors.Is(err, nspd.ErrEmpty) {
		return tileDone
	}
	if errors.Is(err, nspd.ErrTooBig) {
		if tile.Tiny() {
			log.Printf("catalog tile stayed too big")
			return tileMiss
		}
		for _, part := range tile.Quarters() {
			encoded, marshalErr := json.Marshal(part)
			if marshalErr != nil {
				return tileMiss
			}
			if pushErr := db.Redis.LPush(ctx, queueKey, string(encoded)).Err(); pushErr != nil {
				log.Printf("catalog split: %v", pushErr)
				return tileMiss
			}
		}
		return tileDone
	}
	if err != nil {
		if tile.Try < 2 {
			tile.Try++
			encoded, marshalErr := json.Marshal(tile)
			if marshalErr == nil {
				_ = db.Redis.RPush(ctx, queueKey, string(encoded)).Err()
			}
			log.Printf("catalog tile retry: %v", err)
			return tileRetry
		}
		log.Printf("catalog tile: %v", err)
		return tileMiss
	}
	for _, plot := range plots {
		if saveErr := saveCatalogPlot(plot); saveErr != nil {
			log.Printf("catalog save: %v", saveErr)
			return tileMiss
		}
	}
	return tileDone
}

func status() walkStatus {
	ctx := context.Background()
	state, err := db.Redis.Get(ctx, stateKey).Result()
	if err != nil || state == "" {
		state = "idle"
	}
	if busy() {
		state = "running"
	}
	left, _ := db.Redis.LLen(ctx, queueKey).Result()
	message, _ := db.Redis.Get(ctx, messageKey).Result()
	var saved int64
	_ = db.DB.Raw("SELECT COUNT(*) FROM parcels").Scan(&saved).Error
	return walkStatus{State: state, Saved: saved, Left: left, Message: message}
}

func busy() bool {
	walkMu.Lock()
	defer walkMu.Unlock()
	return walking
}

func moderator(w http.ResponseWriter, r *http.Request) (models.User, bool) {
	user, ok := auth.RequireUser(w, r)
	if !ok {
		return models.User{}, false
	}
	if user.Role == nil || user.Role.Code != models.RoleModerator {
		writeError(w, http.StatusForbidden, "обход каталога доступен модератору")
		return models.User{}, false
	}
	return user, true
}
