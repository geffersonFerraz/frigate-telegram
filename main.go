package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/go-telegram/bot"
	"github.com/oldtyt/frigate-telegram/internal/config"
	"github.com/oldtyt/frigate-telegram/internal/frigate"
	"github.com/oldtyt/frigate-telegram/internal/log"
)

// FrigateEvents is frigate events struct
var FrigateEvents frigate.EventsStruct

// FrigateEvent is frigate event struct
var FrigateEvent frigate.EventStruct

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.LogFunc()
	// Get config
	conf := config.New()

	// Prepare startup msg
	startupMsg := "Starting frigate-telegram.\n"
	startupMsg += "Frigate URL:  " + conf.FrigateURL + "\n"
	log.Info.Println(startupMsg)

	opts := []bot.Option{}

	// Initializing telegram bot
	b, err := bot.New(conf.TelegramBotToken, opts...)
	if err != nil {
		log.Error.Fatalln("Error initalizing telegram bot: " + err.Error())
	}
	go b.Start(ctx)

	// Send startup msg. conf.TelegramErrorChatID, startupMsg))
	helloMsg := &bot.SendMessageParams{
		ChatID: conf.TelegramErrorChatID,
		Text:   startupMsg,
	}
	b.SendMessage(ctx, helloMsg)

	// Starting ping command handler(healthcheck)

	FrigateEventsURL := conf.FrigateURL + "/api/events"

	if conf.SendTextEvent {
		go frigate.NotifyEvents(b, FrigateEventsURL)
	}

	if conf.SendInProgressEvent {
		go frigate.NotifyInProgressEvents(b, FrigateEventsURL)
	}

	for {
		go frigate.DefaultEventsLoop(b, FrigateEventsURL)
		time.Sleep(time.Duration(conf.SleepTime) * time.Second)
		if time.Now().Second()%10 == 0 {
			log.Debug.Println("Sleeping for " + strconv.Itoa(conf.SleepTime) + " seconds.")
		}
	}

}
