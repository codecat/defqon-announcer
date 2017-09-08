package main

import "os"
import "os/signal"
import "syscall"
import "time"
import "fmt"
import "net"
import "bufio"
import "strings"
import "reflect"
import "unsafe"

import "github.com/codecat/go-libs/log"
import "github.com/bwmarrin/discordgo"
import "github.com/hajimehoshi/go-mp3"
import "gopkg.in/hraban/opus.v2"
//import "github.com/hajimehoshi/oto"

var keepRunning = true

var lastItem int = -1
var lastItemPreAnnounced int = -1

type NotifyItem struct {
	Name string
	Day int
	Hour int
	UserID string
}
var notifySets []NotifyItem

func getNotifyUsers(scheduleItem *ScheduleItem) []*NotifyItem {
	ret := []*NotifyItem{}
	for i := range notifySets {
		item := &notifySets[i]
		if item.Name == scheduleItem.Name && item.Day == scheduleItem.Time.Day && item.Hour == scheduleItem.Time.Hour {
			ret = append(ret, item)
		}
	}
	return ret
}

func addNotifyUser(scheduleItem *ScheduleItem, userID string) bool {
	for _, item := range notifySets {
		if item.Name == scheduleItem.Name && item.Day == scheduleItem.Time.Day && item.Hour == scheduleItem.Time.Hour && item.UserID == userID {
			return false
		}
	}
	notifySets = append(notifySets, NotifyItem{ scheduleItem.Name, scheduleItem.Time.Day, scheduleItem.Time.Hour, userID })
	return true
}

func getCurrentScheduleItem() *ScheduleItem {
	if lastItem == -1 {
		return nil
	}
	return &schedule.Items[lastItem]
}

func getNextScheduleItem() *ScheduleItem {
	if lastItem + 1 < len(schedule.Items) {
		return &schedule.Items[lastItem + 1]
	}
	return nil
}

func isNextItemPreAnnounced() bool {
	return lastItemPreAnnounced == lastItem + 1
}

func setNextItemPreAnnounced() {
	lastItemPreAnnounced = lastItem + 1
}

func checkForNewScheduleItem() bool {
	t := nowSeconds() * 60 + time.Now().Second()

	ret := false

	for i := lastItem + 1; i < len(schedule.Items); i++ {
		item := schedule.Items[i]
		// Q-Dance crew is 40 seconds late on average.
		if t >= item.TimeSeconds() * 60 + 40 {
			lastItem = i
			ret = true
			log.Info("New item: %s", item.Name)
		}
	}

	return ret
}

func nowSeconds() int {
	t := time.Now()
	return (t.Day() * 1440) + (t.Hour() * 60) + t.Minute()
}

func main() {
	log.Open(log.CatDebug, log.CatFatal)

	if !loadConfig() {
		return
	}

	discord, err := discordgo.New("Bot " + config.Discord.Token)
	if err != nil {
		log.Fatal("Couldn't create Discord API: %s", err.Error())
		return
	}

	discord.AddHandler(messageCreate)
	err = discord.Open()
	if err != nil {
		log.Fatal("Couldn't open Discord connection: %s", err.Error())
		return
	}

	log.Info("Bot running")

	if !loadSchedule() {
		return
	}

	checkForNewScheduleItem()

	/*
	if config.Discord.Voice.Guild != "" && config.Discord.Voice.Channel != "" {
		go botStream(discord)
	}
	*/

	go botTick(discord)

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	keepRunning = false

	discord.Close()
}

func isAdmin(u *discordgo.User) bool {
	for _, a := range config.Discord.Admins {
		if u.ID == a {
			return true
		}
	}
	return false
}

func sendMessage(s *discordgo.Session, channelID string, msg string) {
	s.ChannelMessageSend(channelID, ":musical_note: " + msg)
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

type findItemCallback func(*ScheduleItem)

func findItem(query string, cb findItemCallback) bool {
	query = strings.Trim(strings.ToLower(query), " ")

	if len(query) < 3 {
		return false
	}

	found := false
	for _, item := range schedule.Items {
		if !strings.Contains(strings.ToLower(item.Name), query) {
			continue
		}
		found = true
		cb(&item)
	}
	return found
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if m.Content == ".current" {
		item := getCurrentScheduleItem()
		if item == nil {
			sendMessage(s, m.ChannelID, "There is currently no set playing on stream.")
		} else {
			sendMessage(s, m.ChannelID, fmt.Sprintf("Now playing: %s", formatAnnounceArtist(item)))
		}
	}

	if m.Content == ".next" {
		item := getNextScheduleItem()
		if item == nil {
			sendMessage(s, m.ChannelID, "There is no set up next.")
		} else {
			sendMessage(s, m.ChannelID, fmt.Sprintf("Next up (at %d:%02d CEST): %s", item.Time.Hour, item.Time.Minute, formatAnnounceArtist(item)))
		}
	}

	if m.Content == ".radio" {
		sendMessage(s, m.ChannelID, "Tune in to the Q-BASE stream: <http://radio.q-dance.com/>")
	}

	if m.Content == ".schedule" || m.Content == ".timetable" {
		nextMessage := "Next 5 sets:\n"
		for i := lastItem + 1; i < min(lastItem + 6, len(schedule.Items)); i++ {
			item := schedule.Items[i]
			nextMessage += fmt.Sprintf("- **%d:%02d** CEST: %s\n", item.Time.Hour, item.Time.Minute, formatAnnounceArtist(&item))
		}
		sendMessage(s, m.ChannelID, strings.Trim(nextMessage, "\n"))
	}

	if m.Content == ".github" {
		sendMessage(s, m.ChannelID, "This bot is open source: <https://github.com/codecat/defqon-announcer>")
	}

	if m.Content == ".song" || m.Content == ".songname" {
		sendMessage(s, m.ChannelID, "Darude - Sandstorm")
	}

	if m.Content == ".time" {
		t := time.Now()
		sendMessage(s, m.ChannelID, fmt.Sprintf("The local time is: **%d:%02d**", t.Hour(), t.Minute()))
	}

	parse := strings.SplitN(m.Content, " ", 2)
	if parse[0] == ".find" && len(parse) == 2 {
		found := findItem(parse[1], func(item *ScheduleItem) {
			if nowSeconds() < item.TimeSeconds() {
				sendMessage(s, m.ChannelID, fmt.Sprintf("%s is at **%d:%02d** CEST!", formatAnnounceArtist(item), item.Time.Hour, item.Time.Minute))
			} else {
				sendMessage(s, m.ChannelID, fmt.Sprintf("%s has already played at %d:%02d CEST.", formatAnnounceArtist(item), item.Time.Hour, item.Time.Minute))
			}
		})

		if !found {
			sendMessage(s, m.ChannelID, "I found nothing :frowning:")
		}
	} else if parse[0] == ".notify" && len(parse) == 2 {
		//TODO: .unnotify
		found := findItem(parse[1], func(item *ScheduleItem) {
			if addNotifyUser(item, m.Author.ID) {
				sendMessage(s, m.ChannelID, fmt.Sprintf("<@%s>, signed up! You will be notified for %s", m.Author.ID, formatAnnounceArtist(item)))
			} else {
				sendMessage(s, m.ChannelID, fmt.Sprintf("<@%s>, you already signed up to be notified for %s", m.Author.ID, formatAnnounceArtist(item)))
			}
		})

		if !found {
			sendMessage(s, m.ChannelID, "I found nothing :frowning:")
		}
	}

	if isAdmin(m.Author) {
		if m.Content == ".restartStream" {
			go botStream(s)
		}
	}
}

func formatAnnounceArtist(item *ScheduleItem) string {
	ret := fmt.Sprintf("**%s**", item.Name)
	switch item.Color {
		case "blue": ret += " <:blue:327107850650386432>"
		case "red": ret += " <:red:327107857537302528>"
		case "uv": ret += " <:uv:327107857981898754>"
		case "magenta": ret += " <:magenta:327107856572612618>"
		case "white": ret += " <:white:327107857789091841>"
		case "black": ret += " <:black:327107849572319243>"
	}
	if !item.Live {
		ret += " (recorded earlier in the day)"
	}
	return ret
}

func formatAnnounceNow(item *ScheduleItem) string {
	return fmt.Sprintf(":red_circle: NOW LIVE on stream: %s", formatAnnounceArtist(item))
}

func formatAnnounceSoon(item *ScheduleItem) string {
	return fmt.Sprintf(":clock1: Next up in 5 minutes: %s", formatAnnounceArtist(item))
}

func notifyUsers(s *discordgo.Session, item *ScheduleItem) {
	notifyMessage := ""
	notifies := getNotifyUsers(item)
	for _, notifyItem := range notifies {
		if notifyItem.Name == item.Name {
			notifyMessage += fmt.Sprintf("<@%s>, ", notifyItem.UserID)
		}
	}
	notifyMessage = strings.Trim(notifyMessage, ", ")
	if notifyMessage != "" {
		sendMessage(s, config.Discord.Announce.Channel, ":warning: " + notifyMessage)
	}
}

func botTick(s *discordgo.Session) {
	for keepRunning {
		log.Trace("Checking..")

		if checkForNewScheduleItem() {
			item := getCurrentScheduleItem()
			if item != nil {
				sendMessage(s, config.Discord.Announce.Channel, formatAnnounceNow(item))
				notifyUsers(s, item)
			}
		}

		nextItem := getNextScheduleItem()
		if nextItem != nil && !isNextItemPreAnnounced() {
			nextItemDayMins := nextItem.TimeSeconds()
			currentDayMins := nowSeconds()
			if currentDayMins == nextItemDayMins - 5 {
				sendMessage(s, config.Discord.Announce.Channel, formatAnnounceSoon(nextItem))
				setNextItemPreAnnounced()
				log.Info("New next item: %s", nextItem.Name)
				notifyUsers(s, nextItem)
			}
		}

		time.Sleep(1000 * time.Millisecond)
	}

	log.Info("Shutting down..")
}

func botStream(s *discordgo.Session) {
	log.Info("Joining voice channel")
	voice, err := s.ChannelVoiceJoin(config.Discord.Voice.Guild, config.Discord.Voice.Channel, false, false)
	if err != nil {
		log.Error("Failed to join voice channel: %s", err.Error())
		return
	}
	defer voice.Disconnect()

	if voice.Speaking(true) != nil {
		log.Error("Failed to start speaking: %s", err.Error())
		return
	}
	defer voice.Speaking(false)

	log.Info("Opening Q-Dance stream")
	conn, err := net.Dial("tcp", "audio.true.nl:80")
	if err != nil {
		log.Error("Failed to connect to audio server")
		return
	}
	defer conn.Close()

	fmt.Fprintf(conn, "GET /qdance-hard HTTP/1.1\n")
	fmt.Fprintf(conn, "Host: audio.true.nl\n")
	fmt.Fprintf(conn, "User-Agent: Reddit /r/hardstyle Bot\n")
	fmt.Fprintf(conn, "Accept: */*\n")
	fmt.Fprintf(conn, "\n")

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			log.Error("Failed reading header string")
			return
		}
		line = strings.Trim(line, "\r\n")
		if line == "" {
			break
		}
		log.Debug("Header: \"%s\"", line)
	}

	d, err := mp3.Decode(conn)
	if err != nil {
		log.Error("Decode error: %s", err.Error())
		return
	}

	/*
	soundPlayer, err := oto.NewPlayer(d.SampleRate(), 2, 2, 65536)
	if err != nil {
		fmt.Printf("Player error: %s\n", err.Error())
		return
	}
	defer soundPlayer.Close()
	*/

	var opusSampleRate = 48000
	const opusChannels = 2
	const opusFrameTime = 20 // 60, 40, 20, 10, 5, 2.5

	enc, err := opus.NewEncoder(opusSampleRate, opusChannels, opus.AppAudio)
	if err != nil {
		log.Error("Failed to create Opus encoder: %s", err.Error())
		return
	}

	opusBuffer := make([]byte, 1000)
	for {
		time.Sleep(opusFrameTime * time.Millisecond)

		// 60ms opus frame slice length = (44100.0 / 1000.0 * 60) * 2
		// plus another * 2 because we need int16 count
		pcm := make([]byte, int(float32(opusSampleRate) / 1000.0 * float32(opusFrameTime) * float32(opusChannels)) * 2)
		if float32(len(pcm) / 2 / opusChannels * 1000 / opusSampleRate) != opusFrameTime {
			log.Error("Invalid frame size: %d", len(pcm))
			return
		}

		for decRead := 0; decRead < len(pcm); {
			n, err := d.Read(pcm[decRead:])
			if err != nil {
				log.Error("Error decoding MP3 data: %s", err.Error())
				return
			}
			decRead += n
		}

		//soundPlayer.Write(pcm)

		pcmHeader := *(*reflect.SliceHeader)(unsafe.Pointer(&pcm))
		pcmHeader.Len /= 2
		pcmHeader.Cap /= 2
		pcmInt16 := *(*[]int16)(unsafe.Pointer(&pcmHeader))

		n, err := enc.Encode(pcmInt16, opusBuffer)
		if err != nil {
			log.Error("Error encoding Opus data: %s", err.Error())
			return
		}

		voice.OpusSend <- opusBuffer[:n]
	}
}
