package frigate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oldtyt/frigate-telegram/internal/config"
	"github.com/oldtyt/frigate-telegram/internal/log"
	"github.com/oldtyt/frigate-telegram/internal/redis"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type EventsStruct []struct {
	Box    interface{} `json:"box"`
	Camera string      `json:"camera"`
	Data   struct {
		Attributes []interface{} `json:"attributes"`
		Box        []float64     `json:"box"`
		Region     []float64     `json:"region"`
		Score      float64       `json:"score"`
		TopScore   float64       `json:"top_score"`
		Type       string        `json:"type"`
	} `json:"data"`
	EndTime            *float64    `json:"end_time"`
	FalsePositive      interface{} `json:"false_positive"`
	HasClip            bool        `json:"has_clip"`
	HasSnapshot        bool        `json:"has_snapshot"`
	ID                 string      `json:"id"`
	Label              string      `json:"label"`
	PlusID             interface{} `json:"plus_id"`
	RetainIndefinitely bool        `json:"retain_indefinitely"`
	StartTime          float64     `json:"start_time"`
	SubLabel           []any       `json:"sub_label"`
	Thumbnail          string      `json:"thumbnail"`
	TopScore           interface{} `json:"top_score"`
	Zones              []any       `json:"zones"`
}

type EventStruct struct {
	Box    interface{} `json:"box"`
	Camera string      `json:"camera"`
	Data   struct {
		Attributes []interface{} `json:"attributes"`
		Box        []float64     `json:"box"`
		Region     []float64     `json:"region"`
		Score      float64       `json:"score"`
		TopScore   float64       `json:"top_score"`
		Type       string        `json:"type"`
	} `json:"data"`
	EndTime            *float64    `json:"end_time"`
	FalsePositive      interface{} `json:"false_positive"`
	HasClip            bool        `json:"has_clip"`
	HasSnapshot        bool        `json:"has_snapshot"`
	ID                 string      `json:"id"`
	Label              string      `json:"label"`
	PlusID             interface{} `json:"plus_id"`
	RetainIndefinitely bool        `json:"retain_indefinitely"`
	StartTime          float64     `json:"start_time"`
	SubLabel           []any       `json:"sub_label"`
	Thumbnail          string      `json:"thumbnail"`
	TopScore           interface{} `json:"top_score"`
	Zones              []any       `json:"zones"`
}

var Events EventsStruct
var Event EventStruct

func NormalizeTagText(text string) string {
	var alphabetCheck = regexp.MustCompile(`^[A-Za-z]+$`)
	var NormalizedText []string
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		wordString := fmt.Sprintf("%c", runes[i])
		if _, err := strconv.Atoi(wordString); err == nil {
			NormalizedText = append(NormalizedText, wordString)
		}
		if alphabetCheck.MatchString(wordString) {
			NormalizedText = append(NormalizedText, wordString)
		}
	}
	return strings.Join(NormalizedText, "")
}

func GetTagList(Tags []any) []string {
	var my_tags []string
	for _, zone := range Tags {
		fmt.Println(zone)
		if zone != nil {
			my_tags = append(my_tags, NormalizeTagText(zone.(string)))
		}
	}
	return my_tags
}

func ErrorSend(TextError string, b *bot.Bot, EventID string) {
	conf := config.New()
	TextError += "\nEventID: " + EventID

	helloMsg := &bot.SendMessageParams{
		ChatID: conf.TelegramErrorChatID,
		Text:   TextError,
	}
	log.Debug.Println("Error send: " + TextError)

	_, err := b.SendMessage(context.Background(), helloMsg)
	if err != nil {
		log.Error.Println("Error sending message: " + err.Error())
	}
}

func SaveThumbnail(EventID string, Thumbnail string, b *bot.Bot) string {
	// Decode string Thumbnail base64
	dec, err := base64.StdEncoding.DecodeString(Thumbnail)
	if err != nil {
		ErrorSend("Error when base64 string decode: "+err.Error(), b, EventID)
	}

	// Generate uniq filename
	filename := "/tmp/" + EventID + ".jpg"
	f, err := os.Create(filename)
	if err != nil {
		ErrorSend("Error when create file: "+err.Error(), b, EventID)
	}
	defer f.Close()
	if _, err := f.Write(dec); err != nil {
		ErrorSend("Error when write file: "+err.Error(), b, EventID)
	}
	if err := f.Sync(); err != nil {
		ErrorSend("Error when sync file: "+err.Error(), b, EventID)
	}
	return filename
}

func GetEvents(FrigateURL string, b *bot.Bot, SetBefore bool, OnlyInProgress bool) EventsStruct {
	conf := config.New()

	FrigateURL = FrigateURL + "?limit=" + strconv.Itoa(conf.FrigateEventLimit)

	if SetBefore {
		timestamp := time.Now().UTC().Unix()
		timestamp = timestamp - int64(conf.EventBeforeSeconds)
		FrigateURL = FrigateURL + "&before=" + strconv.FormatInt(timestamp, 10)
	}

	if OnlyInProgress {
		FrigateURL = FrigateURL + "&in_progress=1"
	}

	if time.Now().Second()%10 == 0 {
		log.Debug.Println("Geting events from Frigate via URL: " + FrigateURL)
	}

	// Request to Frigate
	resp, err := http.Get(FrigateURL)
	if err != nil {
		ErrorSend("Error get events from Frigate, error: "+err.Error(), b, "ALL")
		return nil
	}
	if resp == nil || resp.Body == nil {
		ErrorSend("Error get events from Frigate, resp.Body is nil! URL: "+FrigateURL, b, "ALL")
		return nil
	}
	defer resp.Body.Close()
	// Check response status code
	if resp.StatusCode != 200 {
		ErrorSend(fmt.Sprintf("Response status != 200, when getting events from Frigate. Was %d.\nExit.", resp.StatusCode), b, "ALL")
		return nil
	}

	// Read data from response
	byteValue, err := io.ReadAll(resp.Body)
	if err != nil {
		ErrorSend("Can't read JSON: "+err.Error(), b, "ALL")
		return nil
	}

	// Parse data from JSON to struct
	err1 := json.Unmarshal(byteValue, &Events)
	if err1 != nil {
		ErrorSend("Error unmarshal json: "+err1.Error()+" URL: "+FrigateURL, b, "ALL")
		if e, ok := err.(*json.SyntaxError); ok {
			log.Info.Println("syntax error at byte offset " + strconv.Itoa(int(e.Offset)) + " URL: " + FrigateURL)
		}
		log.Info.Println("Exit. URL: " + FrigateURL)
		return nil
	}

	// Return Events
	return Events
}

func SaveClip(EventID string, b *bot.Bot) string {
	// Get config
	conf := config.New()

	// Generate clip URL
	ClipURL := conf.FrigateURL + "/api/events/" + EventID + "/clip.mp4"

	// Generate uniq filename
	filename := "/tmp/" + EventID + ".mp4"

	// Create clip file
	f, err := os.Create(filename)
	if err != nil {
		ErrorSend("Error when create file: "+err.Error(), b, EventID)
	}
	defer f.Close()

	// Download clip file
	resp, err := http.Get(ClipURL)
	if err != nil {
		ErrorSend("Error clip download: "+err.Error(), b, EventID)
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		ErrorSend("Return bad status: "+resp.Status, b, EventID)
	}

	// Writer the body to file
	_, err = io.Copy(f, resp.Body)
	if err != nil {
		ErrorSend("Error clip write: "+err.Error(), b, EventID)
	}
	return filename
}

func SendMessageEvent(FrigateEvent EventStruct, b *bot.Bot, InProgress bool) {
	// Get config
	conf := config.New()
	ctx := context.Background()
	watchDogPrefix := ""
	if InProgress {
		watchDogPrefix = "WatchDog_"
	}

	if !InProgress && FrigateEvent.EndTime == nil {
		return
	}

	redis.AddNewEvent(watchDogPrefix+FrigateEvent.ID, "InWork", time.Duration(60)*time.Second)

	// Prepare text message
	text := "*Event*\n"
	text += "┣*Camera*\n┗ #" + NormalizeTagText(FrigateEvent.Camera) + "\n"
	text += "┣*Label*\n┗ #" + NormalizeTagText(FrigateEvent.Label) + "\n"
	if FrigateEvent.SubLabel != nil {
		text += "┣*SubLabel*\n┗ #" + strings.Join(GetTagList(FrigateEvent.SubLabel), ", #") + "\n"
	}
	t_start := time.Unix(int64(FrigateEvent.StartTime), 0)
	text += fmt.Sprintf("┣*Start time*\n┗ `%s`\n", t_start.Format("15:04:05 02/01/2006"))
	if FrigateEvent.EndTime == nil {
		text += "┣*End time*\n┗ `In progress`\n"
	} else {
		t_end := time.Unix(int64(*FrigateEvent.EndTime), 0)
		text += fmt.Sprintf("┣*End time*\n┗ `%s`\n", t_end.Format("15:04:05 02/01/2006"))
	}
	if !conf.SmallEvent {
		text += fmt.Sprintf("┣*Top score*\n┗ `%f", (FrigateEvent.Data.TopScore*100)) + "%`\n"
		text += "┣*Event id*\n┗ `" + FrigateEvent.ID + "`\n"
		text += "┣*Zones*\n┗ #" + strings.Join(GetTagList(FrigateEvent.Zones), ", #") + "\n"
		text += "*URLs*\n"
		text += "┣[Events](" + conf.FrigateExternalURL + "/events?cameras=" + FrigateEvent.Camera + "&labels=" + FrigateEvent.Label + "&zones=" + strings.Join(GetTagList(FrigateEvent.Zones), ",") + ")\n"
		text += "┣[General](" + conf.FrigateExternalURL + ")\n"
		text += "┗[Source clip](" + conf.FrigateExternalURL + "/api/events/" + FrigateEvent.ID + "/clip.mp4)\n"
	}
	// Save thumbnail
	FilePathThumbnail := SaveThumbnail(FrigateEvent.ID, FrigateEvent.Thumbnail, b)

	filePathThumbnail, err := os.Open(FilePathThumbnail)
	if err != nil {
		ErrorSend("Error opening clip file: "+err.Error(), b, FrigateEvent.ID)
	}
	defer filePathThumbnail.Close()
	defer os.Remove(FilePathThumbnail)

	thumb := &models.InputMediaPhoto{
		Media:           "attach://" + FilePathThumbnail,
		MediaAttachment: filePathThumbnail,
		Caption:         text,
	}
	medias := []models.InputMedia{
		thumb,
	}

	var video *models.InputMediaVideo
	if FrigateEvent.HasClip && FrigateEvent.EndTime != nil {
		FilePathClip := SaveClip(FrigateEvent.ID, b)

		file, err := os.Open(FilePathClip)
		if err != nil {
			ErrorSend("Error opening clip file: "+err.Error(), b, FrigateEvent.ID)
		}

		fileInfo, err := file.Stat()
		if err != nil {
			ErrorSend("Error getting file info: "+err.Error(), b, FrigateEvent.ID)
		}

		const maxSize = 50 * 1024 * 1024 // 50MB
		if fileInfo.Size() > maxSize {
			err := VerificarFFmpegInstalado()
			if err != nil {
				panic(err)
			}

			options := &VideoSplitOptions{
				MaxSizeBytes: 49 * 1024 * 1024, // 49 MB
				OutputFormat: "mp4",
			}

			multipleFiles, err := SplitVideoWithFFmpeg(file, options)
			if err != nil {
				ErrorSend("Error splitting video: "+err.Error(), b, FrigateEvent.ID)
			}
			for k, v := range multipleFiles {
				video = &models.InputMediaVideo{
					MediaAttachment: v,
					Media:           "attach://" + k,
				}
				medias = append(medias, video)
				defer os.Remove(k)
			}
		} else {
			video = &models.InputMediaVideo{
				MediaAttachment: file,
				Media:           "attach://" + FilePathClip,
			}

			medias = append(medias, video)
		}

		defer file.Close()
		defer os.Remove(FilePathClip)
	}

	mediaMsg := &bot.SendMediaGroupParams{
		ChatID:          conf.TelegramChatID,
		MessageThreadID: getMessageThreadId(FrigateEvent.Camera),
		Media:           medias,
	}
	log.Debug.Println("sending message " + FrigateEvent.Camera)
	_, err = b.SendMediaGroup(ctx, mediaMsg)
	if err != nil {
		log.Error.Println("Error sending media group: " + err.Error())
	}

	var State string
	State = "InProgress"
	if FrigateEvent.EndTime != nil {
		State = "Finished"
	}
	redis.AddNewEvent(watchDogPrefix+FrigateEvent.ID, State, time.Duration(conf.RedisTTL)*time.Second)
}

func StringsContains(MyStr string, MySlice []string) bool {
	for _, v := range MySlice {
		if v == MyStr {
			return true
		}
	}
	return false
}

func ParseEvents(FrigateEvents EventsStruct, b *bot.Bot, WatchDog bool, InProgress bool) {
	conf := config.New()
	RedisKeyPrefix := ""
	if WatchDog || InProgress {
		RedisKeyPrefix = "WatchDog_"
	}
	for Event := range FrigateEvents {
		if !(len(conf.FrigateExcludeCamera) == 1 && conf.FrigateExcludeCamera[0] == "None") {
			if StringsContains(FrigateEvents[Event].Camera, conf.FrigateExcludeCamera) {
				log.Debug.Println("Skip event from camera: " + FrigateEvents[Event].Camera)
				continue
			}
		}
		if !(len(conf.FrigateIncludeCamera) == 1 && conf.FrigateIncludeCamera[0] == "All") {
			if !(StringsContains(FrigateEvents[Event].Camera, conf.FrigateIncludeCamera)) {
				log.Debug.Println("Skip event from camera: " + FrigateEvents[Event].Camera)
				continue
			}
		}
		if redis.CheckEvent(RedisKeyPrefix + FrigateEvents[Event].ID) {
			if WatchDog {
				SendTextEvent(FrigateEvents[Event], b)
			} else {
				go SendMessageEvent(FrigateEvents[Event], b, InProgress)
			}
		}
	}
}

func getMessageThreadId(camera string) int {
	threadList := make(map[string]int)
	threadList["General"] = 0
	threadList["Bolacha"] = 2
	threadList["Rua"] = 3
	threadList["Tras"] = 4
	threadList["RuaMAto"] = 5
	threadList["Portao"] = 26
	threadList["TrasPorta"] = 366
	return threadList[camera]
}

func SendTextEvent(FrigateEvent EventStruct, b *bot.Bot) {
	ctx := context.Background()
	conf := config.New()
	text := "*New event*\n"
	text += "┣*Camera*\n┗ `" + FrigateEvent.Camera + "`\n"
	text += "┣*Label*\n┗ `" + FrigateEvent.Label + "`\n"
	t_start := time.Unix(int64(FrigateEvent.StartTime), 0)
	text += fmt.Sprintf("┣*Start time*\n┗ `%s", t_start) + "`\n"
	if !conf.SmallEvent {
		text += fmt.Sprintf("┣*Top score*\n┗ `%f", (FrigateEvent.Data.TopScore*100)) + "%`\n"
		text += "┣*Event id*\n┗ `" + FrigateEvent.ID + "`\n"
		text += "┣*Zones*\n┗ `" + strings.Join(GetTagList(FrigateEvent.Zones), ", ") + "`\n"
		text += "┣*Event URL*\n┗ " + conf.FrigateExternalURL + "/events?cameras=" + FrigateEvent.Camera + "&labels=" + FrigateEvent.Label + "&zones=" + strings.Join(GetTagList(FrigateEvent.Zones), ",")
	}
	message := &bot.SendMessageParams{
		ChatID:          conf.TelegramChatID,
		Text:            text,
		MessageThreadID: getMessageThreadId(FrigateEvent.Camera),
	}
	_, err := b.SendMessage(ctx, message)
	if err != nil {
		log.Error.Println("Error sending message: " + err.Error())
	}
	redis.AddNewEvent("WatchDog_"+FrigateEvent.ID, "Finished", time.Duration(conf.RedisTTL)*time.Second)
}

func NotifyEvents(b *bot.Bot, FrigateEventsURL string) {
	conf := config.New()
	for {
		FrigateEvents := GetEvents(FrigateEventsURL, b, false, false)
		ParseEvents(FrigateEvents, b, true, false)
		time.Sleep(time.Duration(conf.WatchDogSleepTime) * time.Second)
	}
}

func NotifyInProgressEvents(b *bot.Bot, FrigateEventsURL string) {
	conf := config.New()
	for {
		FrigateEvents := GetEvents(FrigateEventsURL, b, false, true)
		ParseEvents(FrigateEvents, b, false, true)
		time.Sleep(time.Duration(conf.WatchDogSleepTime) * time.Second)
	}
}

func DefaultEventsLoop(b *bot.Bot, FrigateEventsURL string) {
	FrigateEvents := GetEvents(FrigateEventsURL, b, true, false)
	if FrigateEvents == nil {
		return
	}
	ParseEvents(FrigateEvents, b, false, false)
}
