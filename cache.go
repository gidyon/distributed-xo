package main

import (
	"github.com/go-redis/redis"
	"github.com/pkg/errors"
	"strconv"
)

func saveInCache(p *playerInfo, redisClient *redis.Client) error {
	return redisClient.HMSet(p.ID, map[string]interface{}{
		"id":    p.ID,
		"name":  p.Name,
		"state": p.State,
		"won":   p.Won,
		"draw":  p.Draw,
		"lost":  p.Lost,
	}).Err()
}

func getPlayerFromRedis(redisClient *redis.Client, key string) (*playerInfo, error) {
	playerMap, err := redisClient.HGetAll(key).Result()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get from map")
	}
	p := &playerInfo{
		ID:    playerMap["id"],
		Name:  playerMap["name"],
		State: playerMap["state"],
		Won:   0,
		Lost:  0,
		Draw:  0,
	}
	if playerMap["won"] != "" {
		p.Won, err = strconv.Atoi(playerMap["won"])
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert won to int")
		}
	}

	if playerMap["draw"] != "" {
		p.Draw, err = strconv.Atoi(playerMap["draw"])
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert draw to int")
		}
	}

	if playerMap["lost"] != "" {
		p.Lost, err = strconv.Atoi(playerMap["lost"])
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert lost to int")
		}
	}

	return p, nil
}

func getPlayerKey(id string) string {
	return "players:" + id
}
