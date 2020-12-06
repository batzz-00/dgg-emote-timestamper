package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

type DGG struct {
	// messageMap map[string]EmoteContainer
	emoteTracker EmoteContainer
	emoteList    map[string]Emote
	toInsert     []EmoteTimestamp
	Database     Database
}

type EmoteMeta struct {
	Emote     Emote
	Timestamp time.Time
}

type Emote struct {
	Prefix string  `json:"prefix"`
	Twitch string  `json:"twitch"`
	Theme  string  `json:"theme"`
	Image  []Image `json:"image"`
}

type Image struct {
	URL    string `json:"url"`
	Name   string `json:"name"`
	Mime   string `json:"mime"`
	Height int    `json:"height"`
	Width  int    `json:"width"`
}

type EmoteTimestamp struct {
	ID        uuid.UUID `db:"id"`
	Emote     string    `db:"emote"`
	Timestamp time.Time
	Count     int
}

type Quarter map[int]EmoteMeta

type MSG struct {
	Nick      string   `json:"nick"`
	Features  []string `json:"features"`
	Timestamp int64    `json:"timestamp"`
	Data      string   `json:"data"`
}

type EmoteContainer struct {
	EmoteMetaMap map[string][]EmoteMeta
	Lock         sync.Mutex
	DestroyAfter time.Duration
}

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		fmt.Println(err)
		log.Fatal("Environment variables could not be loaded")
	}

	db := Database{}
	db.initDB()
	dgg := DGG{
		// messageMap: make(map[string]EmoteContainer),
		Database: db,
		emoteTracker: EmoteContainer{
			EmoteMetaMap: make(map[string][]EmoteMeta),
			DestroyAfter: time.Second * 10,
		},
		emoteList: make(map[string]Emote),
	}
	dgg.fetchEmotes()
	dgg.listen()
}

func (emoteContainer *EmoteContainer) addEmote(emoteMeta EmoteMeta) {
	emoteContainer.Lock.Lock()
	defer emoteContainer.Lock.Unlock()
	if _, ok := emoteContainer.EmoteMetaMap[emoteMeta.Emote.Prefix]; ok {
		emoteContainer.EmoteMetaMap[emoteMeta.Emote.Prefix] = append(emoteContainer.EmoteMetaMap[emoteMeta.Emote.Prefix], emoteMeta)
		return
	}
	emoteContainer.EmoteMetaMap[emoteMeta.Emote.Prefix] = []EmoteMeta{emoteMeta}
}

func (emoteContainer *EmoteContainer) CleanupOldEmotes() {
	now := time.Now()
	emoteContainer.Lock.Lock()
	defer emoteContainer.Lock.Unlock()
	for i := range emoteContainer.EmoteMetaMap {
		keep := []EmoteMeta{}
		for _, emoteMeta := range emoteContainer.EmoteMetaMap[i] {
			if now.Sub(emoteMeta.Timestamp) < emoteContainer.DestroyAfter { // hmm
				keep = append(keep, emoteMeta)
			}
		}
		if len(keep) == 0 {
			delete(emoteContainer.EmoteMetaMap, i)
		} else {
			emoteContainer.EmoteMetaMap[i] = keep
		}
	}
}

func (emoteContainer *EmoteContainer) ToString() string {
	str := ""
	emoteContainer.Lock.Lock()
	defer emoteContainer.Lock.Unlock()

	keys := make([]string, 0, len(emoteContainer.EmoteMetaMap))
	for k := range emoteContainer.EmoteMetaMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		str += fmt.Sprintf("%s(%d), ", emoteContainer.EmoteMetaMap[k][0].Emote.Prefix, len(emoteContainer.EmoteMetaMap[k]))
	}
	str = strings.TrimSuffix(str, ", ")
	return str
}

func earliestTimestamp(timestamps []EmoteMeta) time.Duration {
	now := time.Now()
	// var earliest time.Millisecond
	var earliest time.Duration
	for _, stamp := range timestamps {
		if earliest == 0 {
			earliest = now.Sub(stamp.Timestamp)
			continue
		}
		curDif := now.Sub(stamp.Timestamp)
		if curDif > earliest {
			earliest = curDif
		}
	}
	return earliest
}

func (emoteContainer *EmoteContainer) CheckTimestamps() []EmoteTimestamp {
	Timestamps := []EmoteTimestamp{}
	emoteContainer.Lock.Lock()
	defer emoteContainer.Lock.Unlock()
	for _, meta := range emoteContainer.EmoteMetaMap {
		if len(meta) > 15 {
			earliest := earliestTimestamp(meta) // Only insert if first timestmap in emote list is 8.5s or older so to try and extract as many emotes from list as possible
			if earliest <= time.Millisecond*8500 {
				continue
			}
			fmt.Println(earliest)
			cali, _ := time.LoadLocation("America/Los_Angeles")
			Timestamps = append(Timestamps, EmoteTimestamp{ID: uuid.New(), Emote: meta[0].Emote.Prefix, Timestamp: time.Now().In(cali), Count: len(meta)})
			delete(emoteContainer.EmoteMetaMap, meta[0].Emote.Prefix)
		}
	}
	return Timestamps
}

func (dgg *DGG) fetchEmotes() error {
	// version?
	url := fmt.Sprintf("https://cdn.destiny.gg/2.24.1/emotes/emotes.json?_=%d", time.Now().UnixNano())
	resp, err := http.Get(url)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	emotes := []Emote{}
	err = json.Unmarshal(body, &emotes)
	for _, emote := range emotes {
		dgg.emoteList[emote.Prefix] = emote
	}

	return nil
}

func (EmoteTimestamp *EmoteTimestamp) ToSlice() []interface{} {
	slice := make([]interface{}, 4)
	slice[0] = EmoteTimestamp.ID
	slice[1] = EmoteTimestamp.Emote
	slice[2] = EmoteTimestamp.Count
	slice[3] = EmoteTimestamp.Timestamp

	return slice
}

func (dgg *DGG) Insert() {
	if len(dgg.toInsert) == 0 {
		return
	}
	insertValues := []interface{}{}
	for _, toInsert := range dgg.toInsert {
		insertValues = append(insertValues, toInsert.ToSlice()...)
	}
	sql := fmt.Sprintf("INSERT INTO timestamps (id, emote, count, timestamp) VALUES %s", PrepareBatchValuesPG(4, len(dgg.toInsert)))
	_, err := dgg.Database.db.Exec(sql, insertValues...)
	if err != nil {
		fmt.Println(err)
	}
	dgg.toInsert = []EmoteTimestamp{}
}

func (dgg *DGG) listen() {
	u := url.URL{Scheme: "wss", Host: "chat.destiny.gg:443", Path: "/ws"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {

		for {
			time.Sleep(100 * time.Millisecond)
			dgg.toInsert = append(dgg.toInsert, dgg.emoteTracker.CheckTimestamps()...)
		}
	}()

	go func() {
		for {
			time.Sleep(5 * time.Second)
			dgg.Insert()
		}
	}()

	// go func() {
	// 	for {
	// 		time.Sleep(100 * time.Millisecond)
	// 		fmt.Println(dgg.emoteTracker.ToString())
	// 	}
	// }()

	go func() {
		for {
			time.Sleep(time.Millisecond * 100)
			dgg.emoteTracker.CleanupOldEmotes()
		}
	}()

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			// log.Printf("recv: %s", message)
			if strings.HasPrefix(string(message), "MSG ") {
				var msg MSG
				json.Unmarshal(message[4:], &msg)
				for _, emoteMeta := range dgg.GetEmotesFromMessage(msg.Data) {
					dgg.emoteTracker.addEmote(emoteMeta)
				}
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case t := <-ticker.C:
			err := c.WriteMessage(websocket.TextMessage, []byte(t.String()))
			if err != nil {
				log.Println("write:", err)
				return
			}
		}
	}
}

func (dgg *DGG) GetEmotesFromMessage(message string) []EmoteMeta {
	emotes := []EmoteMeta{}
	splitMessage := strings.Split(message, " ")
	for _, message := range splitMessage {
		if emote, ok := dgg.emoteList[message]; ok {
			emotes = append(emotes, EmoteMeta{Timestamp: time.Now(), Emote: emote})
		}
	}
	return emotes
}

func (dgg *DGG) CleanEmotes() {

}

// wss://chat.destiny.gg/ws
