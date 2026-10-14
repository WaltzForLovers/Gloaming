package events

import "encoding/json"

const (
	TopicWatched = "gloaming.watched"
	TopicCard    = "gloaming.card"
)

type Watched struct {
	UserID    int64 `json:"userId"`
	EpisodeID int64 `json:"episodeId"`
	On        bool  `json:"on"`
}

type Card struct {
	TitleID   int64  `json:"titleId"`
	WikiTitle string `json:"wikiTitle"`
}

func MarshalWatched(userID, episodeID int64, on bool) ([]byte, error) {
	return json.Marshal(Watched{UserID: userID, EpisodeID: episodeID, On: on})
}

func MarshalCard(titleID int64, wikiTitle string) ([]byte, error) {
	return json.Marshal(Card{TitleID: titleID, WikiTitle: wikiTitle})
}
