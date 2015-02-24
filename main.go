package main

import (
	"crypto/tls"
	"fmt"
	"github.com/google/google-api-go-client/calendar/v3"
	"github.com/thoj/go-ircevent"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"time"
)

var timeRFCFormat = "2006-01-02T15:04:00-07:00"
var minutesBeforeNotify = 10

type Config struct {
	Irc struct {
		Host     string `yaml:"host"`
		Nickname string `yaml:"nickname"`
		Password string `yaml:"password"`
		Channel  string `yaml:"channel"`
	} `yaml:"irc"`
	Calendar struct {
		Id    string `yaml:"id"`
		Email string `yaml:"email"`
	} `yaml:"calendar"`
	StartTimeOfDay       string `yaml:"start_time_of_day"`
	NotifyInterval       string `yaml:"notify_interval"`
	parsedNotifyInterval time.Duration
}

func main() {
	configData, err := ioutil.ReadFile("config.yml")
	if err != nil {
		fmt.Printf(`irc:
  host: <your irc server host>
  channel: <irc channel>
  nickname: <nickname>
  password: <password>
calendar:
  id: <calendar id; It's like mail address>
  email: <mail address for outh>
notify_interval: "10m"
start_time_of_day: "10:00:00"
`)
		os.Exit(1)
	}
	config := Config{}
	err = yaml.Unmarshal(configData, &config)
	if err != nil {
		log.Fatalf("config parse error: %s", err)
	}

	log.Printf("%+v", config)

	log.Println("calbot worker start")
	config.IRCLoop()
}

func (c *Config) getTodaysEvents() []*calendar.Event {
	pemData, err := ioutil.ReadFile("key.pem")
	if err != nil {
		log.Fatalf("read key file error: %s", err)
	}

	config := &jwt.Config{
		Email:      c.Calendar.Email,
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

	eventsList := service.Events.List(c.Calendar.Id)
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

func (c *Config) IRCLoop() {
	conn := irc.IRC(c.Irc.Nickname, c.Irc.Nickname)
	//conn.Debug = true
	conn.UseTLS = true
	conn.Password = c.Irc.Password
	conn.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	var err error
	c.parsedNotifyInterval, err = time.ParseDuration(c.NotifyInterval)
	if err != nil {
		log.Fatalf(
			`notify_interval is not duration string.
support units: "ns", "us" (or "µs"), "ms", "s", "m", "h".
`)
	}

	err = conn.Connect(c.Irc.Host)
	if err != nil {
		log.Fatalf("irc connection error: %s", err)
	}

	conn.AddCallback("001", func(e *irc.Event) {
		conn.Join(c.Irc.Channel)
		defer conn.Quit()
		c.notifyLoop(conn)
	})

	conn.Loop()
}

func (c *Config) notifyLoop(conn *irc.Connection) {
	for {
		now := time.Now()
		next := now.Truncate(c.parsedNotifyInterval).Add(c.parsedNotifyInterval)
		events := c.getTodaysEvents()
		for _, event := range events {
			startTime, err := time.Parse(timeRFCFormat, event.Start.DateTime)
			if err != nil {
				log.Printf("time parse error: %s", err)
				continue
			}
			if next.Unix() >= startTime.Unix() && startTime.Unix() >= time.Now().Unix() {
				c.pushEvent(conn, event)
			}
		}
		<-time.After(next.Sub(time.Now()))
	}
}

func (c *Config) pushEvent(conn *irc.Connection, event *calendar.Event) error {
	schedule := fmt.Sprintf("- %s (%s) created by %s\n",
		event.Summary,
		event.Start.DateTime,
		event.Creator.DisplayName,
	)
	conn.Privmsgf(c.Irc.Channel, schedule)

	return nil
}
