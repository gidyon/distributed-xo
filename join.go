package main

import (
	"context"
	"github.com/Pallinder/go-randomdata"
	"github.com/go-redis/redis"
	"net"
	"net/http"
	"sync"
	"time"
)

func (g *game) PlayerJoin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())

	p := &player{
		ctx:         ctx,
		cancel:      cancel,
		mu:          &sync.Mutex{},
		game:        g,
		redisClient: g.redisClient,
		waitingChan: make(chan struct{}, 0),
		info:        &playerInfo{},
	}

	// get user ip address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		logError(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	playerID := "player#" + host

	// check if user has already joined the game and is in set
	exist, err := g.redisClient.SIsMember(playersSet, playerID).Result()
	if err != nil {
		logError(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// player exist in set
	if exist {
		// will update score
		err = g.redisClient.ZAdd(playersZSet, redis.Z{
			Member: playerID,
			Score:  float64(time.Now().UnixNano()),
		}).Err()
		if err != nil {
			logError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// get player from set
		p.info, err = getPlayerFromRedis(g.redisClient, playerID)
		if err != nil {
			logError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if !exist {
		p.info = &playerInfo{
			Name:  randomdata.SillyName(),
			ID:    playerID,
			State: playerStateFree,
			Won:   0,
			Draw:  0,
			Lost:  0,
		}
		// add player to distributed cache
		err := saveInCache(p.info, p.redisClient)
		if err != nil {
			logError(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// expire player data after 12 hours
		p.redisClient.Expire(p.info.ID, 12*time.Hour)
	}

	// upgrade connection to websocket
	p.conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		logError(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = p.WriteError(p.JoinGame())
	if err != nil {
		logError(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	p.Reset()

	// we send information about player
	err = p.WriteJSON(&message{Type: messageWelcome, Payload: p.info})
	if err != nil {
		return
	}

	// we send list of online players
	err = p.conn.WriteJSON(&message{Type: messageAllPlayers, Payload: p.game.FreePlayers()})
	if err != nil {
		return
	}

	// handle all read/write events for this player
	go p.ReadConn()
	go p.ReadChannels()
	<-p.ctx.Done()
	p.LeaveGame()
}
