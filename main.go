package main

import (
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
	"log"
	"io/ioutil"
	"github.com/google/google-api-go-client/calendar/v3"
	"fmt"
	"github.com/thoj/go-ircevent"
	"crypto/tls"
	"time"
)

var calendarId = ""
var ircPassword = ""
var ircHost = ""
var ircNick = ""
var ircChannel = ""
var timeRFCFormat = "2006-01-02T15:04:00-07:00"
var minutesBeforeNotify = 10

func main() {
	log.Println("calbot worker start")
	IRCLoop()
}

func getTodaysEvents() []*calendar.Event {
	pemData, err := ioutil.ReadFile("key.pem")
	if err != nil {
		log.Fatalf("read key file error: %s", err)
	}

    config := &jwt.Config{
		Email: "",
		PrivateKey: pemData,
		Scopes: []string{
			"https://www.googleapis.com/auth/calendar",
		},
		TokenURL: "https://accounts.google.com/o/oauth2/token",
	}

	oauthHttpClient := config.Client(oauth2.NoContext)

	service, err := calendar.New(oauthHttpClient)
	if err != nil {
		log.Fatalf("create calendar service error: %s", err)
	}

	now := time.Now()
	today := now.Truncate(time.Hour * 24)
	todayStr := today.Format(timeRFCFormat)
	tomorrow := today.Add(time.Hour * 24)
	tomorrowStr := tomorrow.Format(timeRFCFormat)

	eventsList := service.Events.List(calendarId)
	eventsList.OrderBy(`startTime`)
	eventsList.TimeMin(todayStr)
	eventsList.TimeMax(tomorrowStr)
	eventsList.SingleEvents(true)
	events, err := eventsList.Do()
	if err != nil {
		log.Fatalf("list events error: %s", err)
	}

	return events.Items
}

func IRCLoop() {
	conn := irc.IRC(ircNick, ircNick)
	//conn.Debug = true
	conn.UseTLS = true
	conn.Password = ircPassword
	conn.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	err := conn.Connect(ircHost)
	if err != nil {
		log.Fatalf("irc connection error: %s", err)
	}

	conn.AddCallback("001", func(e *irc.Event) {
		conn.Join(ircChannel)
		defer conn.Quit()
		notifyLoop(conn)
	})

	conn.Loop()
}

func notifyLoop(conn *irc.Connection) {
	for {
		now := time.Now()
		next := now.Truncate(time.Minute * 10).Add(time.Minute * 10)
		events := getTodaysEvents()
		for _, event := range events {
			startTime, err := time.Parse(timeRFCFormat, event.Start.DateTime)
			if err != nil {
				log.Printf("time parse error: %s", err)
				continue
			}
			if next.Unix() >= startTime.Unix() && startTime.Unix() >= time.Now().Unix() {
				pushEvent(conn, event)
			}
		}
		<- time.After(next.Sub(time.Now()))
	}
}

func pushEvent(conn *irc.Connection, event *calendar.Event) error {
	schedule := fmt.Sprintf("- %s (%s) created by %s\n",
		event.Summary,
		event.Start.DateTime,
		event.Creator.DisplayName,
	)
	conn.Privmsgf(ircChannel, schedule)

	return nil
}


