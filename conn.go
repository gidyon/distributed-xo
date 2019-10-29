package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"time"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

func (p *player) ReadConn() {
	// add some options
	// p.conn.SetReadLimit(maxMessageSize)
	// p.conn.SetReadDeadline(time.Now().Add(pongWait))
	// p.conn.SetPongHandler(func(string) error {
	// 	p.conn.SetReadDeadline(time.Now().Add(pongWait))
	// 	return nil
	// })

	var (
		msg = new(message)
		err error
	)

	for {
		if p.cancelled() {
			return
		}

		err = p.conn.ReadJSON(msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(
				err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure,
			) {
				logError(err)
			}
			logError(err)
			break
		}

		// Process players request
		p.HandleRequest(msg)
	}
}

func (p *player) HandleRequest(msg *message) {
	// handle any panic
	defer func() {
		if err := recover(); err != nil {
			p.WriteErrorString(fmt.Sprint(err))
			p.ExitGameAndPublish()
		}
	}()

	// one request at a time from one player
	// p.mu.Lock()
	// defer p.mu.Unlock()

	var err error

	switch msg.Type {
	case messagePlayerRequestGame: // STEP 1
		// Example payload: REQUESTGAME playerID-wdjbdu938
		playerID, ok := msg.Payload.(string)
		if !ok {
			errMsg := fmt.Sprintf("failed to convert %s payload to string", messagePlayerRequestGame)
			p.WriteErrorString(errMsg)
			break
		}
		// check that you are free
		if p.info.State != playerStateFree {
			p.WriteJSON(&message{
				Type:    messagePlayerBusy,
				Payload: "You",
			})
			break
		}
		// update your state
		p.info.State = playerStateRequesting
		// get the opponent
		opponent, err := getPlayerFromRedis(p.redisClient, playerID)
		if err != nil {
			p.WriteError(err)
			break
		}
		p.opponent = opponent
		// publish request on the opponent channel
		err = p.WriteError(p.PublishRequestGame(playerID))
		if err != nil {
			break
		}
		// time player request for 10 seconds for it to expire
		// go p.TimeOperation(nil, p.SendBusyMessage)
	case messagePlayerRejectGame:
		// Example payload: REJECTGAME playerID-wdjbdu938
		channelID, ok := msg.Payload.(string)
		if !ok {
			errMsg := fmt.Sprintf("failed to convert %s payload to string", messagePlayerRejectGame)
			p.WriteErrorString(errMsg)
			break
		}
		// publish the rejection to opponent channel
		p.WriteError(p.PublishRejectGame(channelID))
		p.Reset()
	case messagePlayerAcceptGame: // STEP 3
		// Example payload: ACCEPTGAME playerID-wdjbdu938
		channelID, ok := msg.Payload.(string)
		if !ok {
			errMsg := fmt.Sprintf("failed to convert %s payload to string", messagePlayerStartGame)
			p.WriteErrorString(errMsg)
			break
		}
		// inform opponent to start game
		err = p.WriteError(p.PublishStartGame(channelID))
		if err != nil {
			break
		}
		p.StartGame()
	case messagePlayerMove: // STEP 5
		// Example payload: PLAYERMOVE box-33
		moveID, ok := msg.Payload.(string)
		if !ok {
			errMsg := fmt.Sprintf("failed to convert %s payload to string", messagePlayerMove)
			p.WriteErrorString(errMsg)
			break
		}
		// publish the move on opponent channel
		p.WriteError(p.PublishMove(moveID))
	case messageGameDraw: // STEP 7
		// Example payload: DRAW
		_, ok := msg.Payload.(string)
		if !ok {
			errMsg := fmt.Sprintf("failed to convert %s payload to string", messageGameDraw)
			p.WriteErrorString(errMsg)
			break
		}
		// publish the draw on opponent channel
		err = p.WriteError(p.PublishGameDraw())
		if err != nil {
			break
		}
		p.info.State = playerStateGameOver
		p.info.Draw++
	case messageGameWon:
		// Example payload: WON winnerID
		playerID, ok := msg.Payload.(string)
		if !ok {
			errMsg := fmt.Sprintf("failed to convert %s payload to string", messageGameWon)
			p.WriteErrorString(errMsg)
			break
		}
		// publish the won on opponent channel
		err = p.WriteError(p.PublishGameWon(playerID))
		if err != nil {
			break
		}
		if playerID == p.info.ID {
			// i won
			p.info.Won++
		} else {
			// i lost
			p.info.Lost++
		}
	case messagePlayerRestartGame: // STEP 8
		// Example payload: RESTARTGAME
		err = p.WriteError(p.PublishGameRestart())
		if err != nil {
			break
		}
		// time client for 10 seconds
		select {
		case <-time.After(10 * time.Second):
			p.ExitGameAndPublish()
		case <-p.waitingChan:
			// notify the client that game can start
			err = p.WriteJSON(&message{
				Type:    messagePlayerStartGame,
				Payload: p.opponent,
			})
			if err != nil {
				break
			}
			// update your state to playing
			p.info.State = playerStatePlaying
		}
	case messagePlayerExitGame:
		p.ExitGameAndPublish()
	}
}
