package main

import (
	"fmt"
)

func (p *player) ReadConn() {
	var (
		msg = new(message)
		err error
	)

	for {
		err = p.conn.ReadJSON(msg)
		if err != nil {
			// cancel game
			p.cancel()
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
		// will wait
		p.InitWaitingChan()
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
		go p.TimeOperation(nil, p.SendBusyMessage)
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
		if p.info.State != playerStatePlaying {
			break
		}
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
		p.InitWaitingChan()
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
		p.WriteError(p.redisClient.HIncrBy(p.info.ID, "draw", 1).Err())
		p.info.State = playerStateGameOver
		p.info.Draw++
	case messageGameWon:
		// Example payload: WON winnerID
		p.InitWaitingChan()
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
			err = p.WriteError(p.redisClient.HIncrBy(p.info.ID, "won", 1).Err())
			if err != nil {
				break
			}
			p.info.Won++
		} else {
			// i lost
			err = p.WriteError(p.redisClient.HIncrBy(p.info.ID, "lost", 1).Err())
			if err != nil {
				break
			}
			p.info.Lost++
		}
		p.info.State = playerStateGameOver
	case messagePlayerRestartGame: // STEP 8
		// Example payload: RESTARTGAME
		err = p.WriteError(p.PublishGameRestart())
		if err != nil {
			break
		}
		// update state of player
		p.info.State = playerStateGameOver
		// time player for 10 sec
		go p.TimeOperation(p.RestartGame, p.ExitGameAndPublish)
	case messagePlayerExitGame:
		p.ExitGameAndPublish()
	}
}
