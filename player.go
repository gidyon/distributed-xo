package main

import (
	"context"
	"github.com/go-redis/redis"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
	"sync"
	"time"
)

type playerInfo struct {
	ID    string
	Name  string
	State string
	Won   int
	Draw  int
	Lost  int
}

type player struct {
	ctx         context.Context
	mu          *sync.Mutex // guards conn
	conn        *websocket.Conn
	game        *game
	waitingChan chan struct{}
	redisClient *redis.Client
	opponent    *playerInfo
	opponentID  string
	info        *playerInfo
}

func (p *player) JoinGame() error {
	// add player to distributed cache
	err := saveInCache(p.info, p.redisClient)
	if err != nil {
		return errors.Wrap(err, "failed to save player in cache")
	}

	// add player id to set
	err = p.redisClient.SAdd(playersSet, p.info.ID).Err()
	if err != nil {
		return errors.Wrap(err, "failed to add player id to set")
	}

	// add player id to sorted set using timestamp as score
	err = p.redisClient.ZAdd(playersZSet, redis.Z{
		Score:  float64(time.Now().UnixNano()),
		Member: p.info.ID,
	}).Err()
	if err != nil {
		return errors.Wrap(err, "failed to add player id to sorted set")
	}

	// notify free players using free players channel
	err = p.PublishJoinFreeChannel()
	if err != nil {
		return errors.Wrap(err, "failed to publish join to players channel")
	}

	return nil
}

func (p *player) LeaveGame() error {
	// delete player from cache
	err := p.redisClient.Del(p.info.ID).Err()
	if err != nil {
		return errors.Wrap(err, "failed to delete player from cache")
	}

	// remove from available players set
	err = p.redisClient.ZRem(playersZSet, p.info.ID).Err()
	if err != nil {
		return errors.Wrap(err, "failed to remove player from set")
	}

	// notify all free players that you are leaving
	err = p.PublishLeftFreeChannel()
	if err != nil {
		return errors.Wrap(err, "failed to publish leave to free players channel")
	}

	return nil
}

func (p *player) cancelled() bool {
	select {
	case <-p.ctx.Done():
		return true
	default:
		return false
	}
}

func (p *player) PublishMessage(channel, pmessage string) error {
	err := p.redisClient.Publish(channel, pmessage).Err()
	if err != nil {
		err0 := p.conn.WriteJSON(&message{
			Type:    messageErrorHappened,
			Payload: err.Error(),
		})
		if err0 != nil {
			logError(err0)
			return err0
		}
		return err
	}
	return nil
}

func (p *player) PublishMessageToGameChannel(pmessage string) error {
	return p.PublishMessage(p.opponent.ID, pmessage)
}

func (p *player) PublishBusyMessage(channel string) error {
	// Example message: PLAYERBUSY:::myid
	return errors.Wrap(
		p.PublishMessage(channel, playerBusy(p.info.ID)),
		"failed to publish busy message",
	)
}

func (p *player) PublishRequestGame(channel string) error {
	// Example message: REQUESTGAME:::myid
	return errors.Wrap(
		p.PublishMessage(channel, playerRequestGame(p.info.ID)),
		"failed to publish request game message",
	)
}

func (p *player) PublishRejectGame(channel string) error {
	// Example payload: REJECTGAME:::myid
	return errors.Wrap(
		p.PublishMessageToGameChannel(playerRejectGame(p.info.ID)),
		"failed to publish reject game message",
	)
}

func (p *player) PublishStartGame(channel string) error {
	// Example payload: STARTGAME:::myid
	return errors.Wrap(
		p.PublishMessageToGameChannel(playerStartGame(p.info.ID)),
		"failed to publish accept game message",
	)
}

func (p *player) PublishMove(moveID string) error {
	return errors.Wrap(
		p.PublishMessageToGameChannel(playerMove(moveID)),
		"failed to publish players move",
	)
}

func (p *player) PublishGameWon(winnerID string) error {
	return errors.Wrap(
		p.PublishMessageToGameChannel(playerWon(winnerID)),
		"failed to publish game won message",
	)
}

func (p *player) PublishGameDraw() error {
	return errors.Wrap(
		p.PublishMessageToGameChannel(messageGameDraw),
		"failed to publish game draw message",
	)
}

func (p *player) PublishGameRestart() error {
	return errors.Wrap(
		p.PublishMessageToGameChannel(messagePlayerRestartGame),
		"failed to publish restart game message",
	)
}

func (p *player) PublishGameExit() error {
	return errors.Wrap(
		p.PublishMessageToGameChannel(messagePlayerExitGame),
		"failed to publish exited game message",
	)
}

func (p *player) PublishJoinFreeChannel() error {
	// Example payload: LEFT:::myid
	return errors.Wrap(
		p.PublishMessage(playersChannel, playerJoin(p.info.ID)),
		"failed to publish new player joined message",
	)
}

func (p *player) PublishLeftFreeChannel() error {
	// Example payload: LEFT:::myid
	return errors.Wrap(
		p.PublishMessage(playersChannel, playerLeft(p.info.ID)),
		"failed to publish new player left message",
	)
}

func (p *player) ExitFreePlayers() error {
	// remove from available players set
	return errors.Wrap(
		p.redisClient.ZRem(playersZSet, p.info.ID).Err(),
		"failed to remove player from free players set",
	)
}

func (p *player) JoinFreePlayers() error {
	// add from available players set
	return errors.Wrap(
		p.redisClient.ZAdd(playersZSet, redis.Z{
			Member: p.info.ID,
			Score:  float64(time.Now().UnixNano()),
		}).Err(),
		"failed to join free players set",
	)
}

func (p *player) Reset() {
	p.CloseWaitingChan()
	// p.opponent = nil
	// p.opponentID = ""
	p.info.State = playerStateFree
}

func (p *player) StartOver() {
	p.Reset()
	p.WriteError(p.PublishJoinFreeChannel())
}

func (p *player) RequestExpired() bool {
	select {
	case <-p.waitingChan:
		return true
	default:
		return false
	}
}

func (p *player) StopWaiting() {
	if !p.RequestExpired() {
		close(p.waitingChan)
	}
}

func (p *player) TimeOperation(successFn, timedOutFn func()) {
	// close waiting chan if not closed
	defer p.CloseWaitingChan()

	p.waitingChan = make(chan struct{})
	select {
	case <-p.waitingChan: // waiting was over
		if successFn != nil {
			successFn()
		}
	case <-time.After(10 * time.Second):
		if timedOutFn != nil {
			timedOutFn()
		}
	}
}

func (p *player) SendBusyMessage() {
	defer p.Reset()

	// send busy message to other player
	err := p.WriteError(p.PublishBusyMessage(p.opponent.ID))
	if err != nil {
		return
	}
	// send busy message to client
	p.WriteJSON(&message{
		Type:    messagePlayerBusy,
		Payload: p.opponent.Name,
	})
}

func (p *player) WriteJSON(msg *message) error {
	p.mu.Lock()
	err := p.conn.WriteJSON(msg)
	p.mu.Unlock()
	logError(err)
	return err
}

func (p *player) WriteError(err error) error {
	if err != nil {
		return p.WriteJSON(&message{Type: messageErrorHappened, Payload: err.Error()})
	}
	return nil
}

func (p *player) WriteErrors(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *player) WriteErrorString(errMsg string) error {
	return p.WriteJSON(&message{Type: messageErrorHappened, Payload: errMsg})
}

func (p *player) StartGame() {
	p.CloseWaitingChan()
	err := p.WriteErrors(p.ExitFreePlayers(), p.PublishLeftFreeChannel())
	if err != nil {
		return
	}
	// notify the client that game can start, send along the opponent
	err = p.WriteJSON(&message{
		Type:    messagePlayerStartGame,
		Payload: p.opponent,
	})
	if err != nil {
		return
	}
	// update your state to playing
	p.info.State = playerStatePlaying
}

func (p *player) ExitGameAndPublish() {
	defer p.Reset()

	// exit game to notify other player
	p.WriteError(p.PublishGameExit())

	logInfo("my name is %s", p.info.Name)
	// join free players
	// publish join
	err := p.WriteErrors(p.JoinFreePlayers(), p.PublishJoinFreeChannel())
	if err != nil {
		return
	}
}

func (p *player) ExitGame() {
	defer p.Reset()

	logInfo("my name is %s", p.info.Name)
	// join free players
	// publish join
	err := p.WriteErrors(p.JoinFreePlayers(), p.PublishJoinFreeChannel())
	if err != nil {
		return
	}
	// notify client
	p.WriteJSON(&message{
		Type:    messagePlayerExitGame,
		Payload: "",
	})
}

func (p *player) CloseWaitingChan() {
	select {
	case <-p.waitingChan:
	default:
		close(p.waitingChan)
	}
}
