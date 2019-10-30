package main

import (
	"github.com/Sirupsen/logrus"
	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"net/http"
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
		g.playersNum++
		if p.State == playerStateFree {
			// add free member
			g.freePlayersNum++
			g.freePlayers[p.ID] = p
		}
	}

	close(g.waitChan)

	// run game
	go g.run()

	return g, nil
}

func (g *game) GetPlayer(playerID string) *playerInfo {
	<-g.waitChan
	p, _ := g.freePlayers[playerID]
	return p
}

func (g *game) FreePlayers() map[string]*playerInfo {
	<-g.waitChan
	logrus.Infoln("free players: ", len(g.freePlayers))
	return g.freePlayers
}

func (g *game) run() {
	pubSub := g.redisClient.Subscribe(playersChannel)
	for {
		msg := <-pubSub.Channel()
		broadCastType, payload := fromBroadCast(msg.Payload)
		switch broadCastType {
		case messagePlayerJoin:
			logrus.Infoln("new player id: ", payload)
			g.waitChan = make(chan struct{})
			p, err := getPlayerFromRedis(g.redisClient, payload)
			if err != nil {
				logrus.Errorln(err)
				break
			}
			logrus.Infoln("new player name: ", p.Name)
			g.newPlayer = p
			g.playersNum++
			g.freePlayersNum++
			g.freePlayers[p.ID] = p
			close(g.waitChan)
		case messagePlayerLeft:
			delete(g.freePlayers, payload)
			g.playersNum--
			g.freePlayersNum--
		}
	}
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

func logError(err error) {
	if err != nil {
		logrus.Errorln(err)
	}
}

func logInfo(format string, args ...interface{}) {
	logrus.Printf(format, args...)
}
