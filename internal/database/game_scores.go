package database

import (
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// GameScore is one player's total in the word game of one chat.
type GameScore struct {
	ChatID int64  `bson:"chat_id"`
	UserID int64  `bson:"user_id"`
	Name   string `bson:"name"`
	Points int    `bson:"points"`
	Wins   int    `bson:"wins"`
}

// AddGameScore adds points (and one correct answer) to a player.
func AddGameScore(chatID, userID int64, name string, points int) error {
	c, cancel := ctx()
	defer cancel()

	_, err := database.Collection("game_scores").UpdateOne(
		c,
		bson.M{"chat_id": chatID, "user_id": userID},
		bson.M{
			"$inc": bson.M{"points": points, "wins": 1},
			"$set": bson.M{"name": name},
		},
		upsertOpt,
	)
	return err
}

// TopGameScores returns the best players of a chat, highest points first.
func TopGameScores(chatID int64, limit int) ([]GameScore, error) {
	c, cancel := ctx()
	defer cancel()

	cur, err := database.Collection("game_scores").Find(
		c,
		bson.M{"chat_id": chatID},
		options.Find().
			SetSort(bson.D{{Key: "points", Value: -1}}).
			SetLimit(int64(limit)),
	)
	if err != nil {
		return nil, err
	}
	var out []GameScore
	if err := cur.All(c, &out); err != nil {
		return nil, err
	}
	return out, nil
}
