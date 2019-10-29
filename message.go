package main

import (
	"fmt"
	"strings"
)

type message struct {
	Type    string
	Payload interface{}
}

const (
	// channels
	playersChannel = "channel:players"
	playersZSet    = "zset:players"
	playersSet     = "set:players"

	// player states
	playerStateFree       = "FREE"
	playerStatePlaying    = "PLAYING"
	playerStateRequesting = "REQUESTING"
	playerStateGameOver   = "GAMEOVER"

	// messages
	messageWelcome           = "WELCOME"
	messageAllPlayers        = "PLAYERS"
	messagePlayerJoin        = "JOIN"
	messagePlayerLeft        = "LEFT"
	messagePlayerRequestGame = "REQUESTGAME"
	messagePlayerRejectGame  = "REJECTGAME"
	messagePlayerAcceptGame  = "ACCEPTGAME"
	messagePlayerStartGame   = "STARTGAME"
	messagePlayerNotPlaying  = "STATENOTPLAYING"
	messagePlayerRestartGame = "RESTARTGAME"
	messagePlayerExitGame    = "PLAYEREXIT"
	messagePlayerBusy        = "PLAYERBUSY"
	messagePlayerMove        = "PLAYERMOVE"
	messageGameOn            = "GAMEON"
	messageGameWon           = "WON"
	messageGameLost          = "LOST"
	messageGameDraw          = "DRAW"
	messageErrorHappened     = "ERROR"
	messageSplit             = ":::"
)

func playerJoin(playerID string) string {
	return fmt.Sprintf("%s%s%s", messagePlayerJoin, messageSplit, playerID)
}

func playerLeft(playerID string) string {
	return fmt.Sprintf("%s%s%s", messagePlayerLeft, messageSplit, playerID)
}

func playerRequestGame(playerID string) string {
	return fmt.Sprintf("%s%s%s", messagePlayerRequestGame, messageSplit, playerID)
}

func playerRejectGame(playerID string) string {
	return fmt.Sprintf("%s%s%s", messagePlayerRejectGame, messageSplit, playerID)
}

func playerStartGame(playerID string) string {
	return fmt.Sprintf("%s%s%s", messagePlayerStartGame, messageSplit, playerID)
}

func playerMove(moveID string) string {
	return fmt.Sprintf("%s%s%s", messagePlayerMove, messageSplit, moveID)
}

func playerWon(winnerID string) string {
	return fmt.Sprintf("%s%s%s", messageGameWon, messageSplit, winnerID)
}

func playerExitGame(playerID string) string {
	return fmt.Sprintf("%s%s%s", messagePlayerExitGame, messageSplit, playerID)
}

func playerBusy(playerID string) string {
	return fmt.Sprintf("%s%s%s", messagePlayerBusy, messageSplit, playerID)
}

// returns the broadcast type and payload
func fromBroadCast(broadcastMsg string) (string, string) {
	ss := strings.Split(broadcastMsg, messageSplit)
	if len(ss) == 0 {
		return "UNKNOWN", ""
	}
	if len(ss) < 2 {
		return ss[0], ""
	}
	return ss[0], ss[1]
}
