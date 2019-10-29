package main

import (
	"github.com/Pallinder/go-randomdata"
	"github.com/Sirupsen/logrus"
	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"net"
	"net/http"
	"sync"
	"time"
)

type game struct {
	redisClient    *redis.Client
	newPlayer      *playerInfo
	freePlayers    map[string]*playerInfo
	waitChan       chan struct{}
	playersNum     int
	freePlayersNum int
}

func newGame(redisClient *redis.Client) (*game, error) {
	if redisClient == nil {
		return nil, errors.New("nil redis client")
	}
	g := &game{
		redisClient: redisClient,
		newPlayer:   &playerInfo{},
		freePlayers: make(map[string]*playerInfo, 0),
		waitChan:    make(chan struct{}, 0),
	}

	// get 500 latest players from redis sorted set
	members, err := redisClient.ZRange(playersZSet, 0, 500).Result()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get free players")
	}

	for _, member := range members {
		p, err := getPlayerFromRedis(redisClient, member)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get player from redis")
		}
		if p.State == playerStateFree {
			// add free member
			g.freePlayers[p.ID] = p
		}
	}

	close(g.waitChan)

	// run game
	go g.run()

	return g, nil
}

func (g *game) NewPlayer() *playerInfo {
	<-g.waitChan
	return g.newPlayer
}

func (g *game) FreePlayers() map[string]*playerInfo {
	return g.freePlayers
}

func (g *game) run() {
	pubSub := g.redisClient.Subscribe(playersChannel)
	for {
		msg := <-pubSub.Channel()
		broadCastType, payload := fromBroadCast(msg.Payload)
		switch broadCastType {
		case messagePlayerJoin:
			g.waitChan = make(chan struct{})
			p, err := getPlayerFromRedis(g.redisClient, payload)
			if err != nil {
				logrus.Errorln(err)
				break
			}
			g.newPlayer = p
			g.playersNum++
			g.freePlayersNum++
			g.freePlayers[p.ID] = p
			close(g.waitChan)
		case messagePlayerLeft:
			delete(g.freePlayers, payload)
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func (g *game) PlayerJoin(w http.ResponseWriter, r *http.Request) {
	p := &player{
		ctx:         r.Context(),
		mu:          &sync.Mutex{},
		game:        g,
		redisClient: g.redisClient,
		waitingChan: make(chan struct{}, 0),
	}

	// get user ip address
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		logrus.Errorln(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	playerID := "player" + host

	// check if user has already joined the game and is in set
	exist, err := g.redisClient.SIsMember(playersSet, playerID).Result()
	if err != nil {
		logrus.Errorln(err)
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
			logrus.Errorln(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// get player from set
		p.info, err = getPlayerFromRedis(g.redisClient, playerID)
		if err != nil {
			logrus.Errorln(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if !exist {
		p.info = &playerInfo{
			ID:    playerID,
			Name:  randomdata.SillyName(),
			State: playerStateFree,
		}
	}

	// player joins the distributed game
	err = p.WriteError(p.JoinGame())
	if err != nil {
		return
	}

	defer logrus.Infoln("player left: ", p.info.Name)

	// upgrade connection to websocket
	p.conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		logrus.Errorln(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// we send information about player
	err = p.WriteJSON(&message{Type: messageWelcome, Payload: p.info})
	if err != nil {
		return
	}

	<-p.game.waitChan

	// we send list of online players
	err = p.conn.WriteJSON(&message{Type: messageAllPlayers, Payload: p.game.FreePlayers()})
	if err != nil {
		return
	}

	// handle all read/write events for this player
	go p.ReadConn()
	p.ReadChannels()
}

func logError(err error) {
	if err != nil {
		logrus.Errorln(err)
	}
}

func logInfo(format string, args ...interface{}) {
	logrus.Printf(format, args...)
}
