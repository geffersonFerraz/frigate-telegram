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
	var isSplit *os.File
	const maxSize = 49 * 1024 * 1024 // 50MB

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

		if fileInfo.Size() > maxSize {
			isSplit = file
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
		ErrorSend("Error sending media group: "+err.Error(), b, FrigateEvent.ID)
		log.Error.Println("Error sending media group: " + err.Error())
	}

	if isSplit != nil {
		err := VerificarFFmpegInstalado()
		if err != nil {
			panic(err)
		}

		options := &VideoSplitOptions{
			MaxSizeBytes: maxSize,
			OutputFormat: "mp4",
		}

		multipleFiles, err := SplitVideoWithFFmpeg(isSplit, options)
		if err != nil {
			ErrorSend("Error splitting video: "+err.Error(), b, FrigateEvent.ID)
		}
		for i, v := range multipleFiles {
			vFile, err := os.Open(v)
			if err != nil {
				ErrorSend("Error opening clip file: "+err.Error(), b, FrigateEvent.ID)
			}
			defer vFile.Close()
			defer os.Remove(v)

			video = &models.InputMediaVideo{
				MediaAttachment: vFile,
				Media:           "attach://" + v,
				Caption:         fmt.Sprintf("This is a part %d of %d\n*Event id*\n `"+FrigateEvent.ID, i+1, len(multipleFiles)),
			}

			mediaMsg := &bot.SendMediaGroupParams{
				ChatID:          conf.TelegramChatID,
				MessageThreadID: getMessageThreadId(FrigateEvent.Camera),
				Media:           []models.InputMedia{video},
			}

			log.Debug.Println("sending part video message " + FrigateEvent.Camera)
			_, err = b.SendMediaGroup(ctx, mediaMsg)
			if err != nil {
				ErrorSend("Error sending media group part: "+err.Error(), b, FrigateEvent.ID)
				log.Error.Println("Error sending media group part: " + err.Error())
			}

		}
	}

	var State string
	State = "InProgress"

	if InProgress && FrigateEvent.EndTime == nil {
		State = "AlreadyInProgress"
	}

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

		if !redis.CheckEvent(RedisKeyPrefix+FrigateEvents[Event].ID) && InProgress && FrigateEvents[Event].EndTime != nil {
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
	text += "┣*Event id*\n┗ `" + FrigateEvent.ID + "`\n"
	text += "┣*Label*\n┗ `" + FrigateEvent.Label + "`\n"
	t_start := time.Unix(int64(FrigateEvent.StartTime), 0)
	text += fmt.Sprintf("┣*Start time*\n┗ `%s", t_start) + "`\n"
	if !conf.SmallEvent {
		text += fmt.Sprintf("┣*Top score*\n┗ `%f", (FrigateEvent.Data.TopScore*100)) + "%`\n"
		text += "┣*Camera*\n┗ `" + FrigateEvent.Camera + "`\n"
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
	// var FrigateEvents EventsStruct
	// byteValue := `[{"area":null,"box":null,"camera":"Rua","data":{"attributes":[],"box":[0.2989583333333333,0.03796296296296296,0.06302083333333333,0.12037037037037036],"region":[0.2453125,0.0,0.16666666666666666,0.2962962962962963],"score":0.76953125,"top_score":0.796875,"type":"object"},"detector_type":"cpu","end_time":1732995090.640684,"false_positive":null,"has_clip":true,"has_snapshot":true,"id":"1732992057.015251-pb90wt","label":"car","model_hash":"115eb60dc73b76e183b6a16b66aa3291","model_type":"ssd","plus_id":null,"ratio":1.0,"region":null,"retain_indefinitely":false,"score":null,"start_time":1732992047.015251,"sub_label":null,"thumbnail":"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","top_score":null,"zones":["ZonaDefault"]}]`
	// stringToJson := []byte(byteValue)
	// // Parse data from JSON to struct
	// err1 := json.Unmarshal(stringToJson, &FrigateEvents)
	// if err1 != nil {
	// 	log.Error.Println("Error parsing JSON: " + err1.Error())
	// }

	FrigateEvents := GetEvents(FrigateEventsURL, b, true, false)
	if FrigateEvents == nil {
		return
	}
	ParseEvents(FrigateEvents, b, false, false)

	// SendMessageEvent(FrigateEvents[0], b, false)
}
