package main

import ()

func (p *player) ReadChannels() {
	// subscribe to channels
	free := p.redisClient.Subscribe(playersChannel)
	own := p.redisClient.Subscribe(p.info.ID)

	var err error

	for {
		select {
		case <-p.ctx.Done():
			logError(p.LeaveGame())
			return
		case msg := <-free.Channel():
			broadMessageType, payload := fromBroadCast(msg.Payload)
			switch broadMessageType {
			case messagePlayerJoin:
				// send the new player to client
				p.WriteJSON(&message{
					Type:    messagePlayerJoin,
					Payload: p.game.NewPlayer(),
				})
			case messagePlayerLeft:
				// send the id of the player to client
				p.WriteJSON(&message{
					Type:    messagePlayerLeft,
					Payload: payload,
				})
			}
		case msg := <-own.Channel():
			broadMessageType, payload := fromBroadCast(msg.Payload)
			switch broadMessageType {
			case messagePlayerRequestGame: // STEP 2
				// check that you are not playing or waiting for other player
				if p.info.State != playerStateFree {
					// publish busy message
					p.WriteError(p.PublishBusyMessage(payload))
					break
				}
				// get opponent
				opponent, err := getPlayerFromRedis(p.redisClient, payload)
				if err != nil {
					p.WriteError(err)
					break
				}
				p.opponent = opponent
				p.info.State = playerStateRequesting
				// notify the client that someone want to play; send along the opponent details
				p.WriteJSON(&message{
					Type:    messagePlayerRequestGame,
					Payload: opponent,
				})
			case messagePlayerBusy:
				p.CloseWaitingChan()
				// tell your client other player is busy
				p.WriteJSON(&message{
					Type:    messagePlayerBusy,
					Payload: p.opponent.Name,
				})
				p.Reset()
			case messagePlayerRejectGame:
				// tell client their game request was rejected
				err = p.WriteJSON(&message{
					Type:    messagePlayerRejectGame,
					Payload: "",
				})
				if err != nil {
					break
				}
				p.Reset()
			case messagePlayerStartGame: // STEP 4
				p.StartGame()
			case messagePlayerMove: // STEP 6
				// we check our state
				if p.info.State != playerStatePlaying {
					// exit game!
					p.ExitGameAndPublish()
					break
				}
				// consume the other player's move by forwarding it to client
				p.WriteJSON(&message{
					Type:    messagePlayerMove,
					Payload: payload,
				})
			case messageGameDraw:
				p.info.State = playerStateGameOver
				p.info.Draw++
				p.WriteJSON(&message{
					Type:    messageGameDraw,
					Payload: "Draw!",
				})
			case messageGameWon:
				p.info.State = playerStateGameOver
				if payload == p.info.ID {
					p.info.Won++
				} else {
					p.info.Lost++
				}
				p.WriteJSON(&message{
					Type:    messageGameWon,
					Payload: payload,
				})
			case messagePlayerRestartGame:
				p.CloseWaitingChan()
			case messagePlayerExitGame:
				p.ExitGame()
			}
		}
	}
}
