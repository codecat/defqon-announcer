package main

import "os"
import "os/signal"
import "syscall"
import "time"
import "fmt"

import "github.com/codecat/go-libs/log"
import "github.com/bwmarrin/discordgo"

var keepRunning = true

var lastItem int = -1
var lastItemPreAnnounced int = -1

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
	t := time.Now()

	ret := false

	for i := lastItem + 1; i < len(schedule.Items); i++ {
		item := schedule.Items[i]
		if t.Day() >= item.Time.Day && t.Hour() >= item.Time.Hour && t.Minute() >= item.Time.Minute {
			lastItem = i
			ret = true
			log.Info("New item: %s", item.Name)
		}
	}

	return ret
}

func main() {
	log.Open(log.CatDebug, log.CatFatal)

	if !loadConfig() {
		return
	}

	discord, err := discordgo.New("Bot " + config.DiscordToken)
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

	go botTick(discord)

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	keepRunning = false

	discord.Close()
}

func sendMessage(s *discordgo.Session, channelID string, msg string) {
	s.ChannelMessageSend(channelID, ":musical_note: " + msg)
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
		sendMessage(s, m.ChannelID, "Tune in to the Defqon stream: <http://radio.q-dance.com/>")
	}

	if m.Content == ".schedule" || m.Content == ".timetable" {
		sendMessage(s, m.ChannelID, "Defqon 1 Timetable: <http://imgur.com/a/8p4dH>")
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

func botTick(s *discordgo.Session) {
	for keepRunning {
		log.Trace("Checking..")

		if checkForNewScheduleItem() {
			item := getCurrentScheduleItem()
			if item != nil {
				sendMessage(s, config.DiscordAnnounceChannel, formatAnnounceNow(item))
			}
		}

		t := time.Now()

		nextItem := getNextScheduleItem()
		if nextItem != nil && !isNextItemPreAnnounced() {
			nextItemDayMins := (nextItem.Time.Day * 1440) + (nextItem.Time.Hour * 60) + nextItem.Time.Minute
			currentDayMins := (t.Day() * 1440) + (t.Hour() * 60) + t.Minute()
			if currentDayMins == nextItemDayMins - 5 {
				sendMessage(s, config.DiscordAnnounceChannel, formatAnnounceSoon(nextItem))
				setNextItemPreAnnounced()
				log.Info("New next item: %s", nextItem.Name)
			}
		}

		time.Sleep(1000 * time.Millisecond)
	}

	log.Info("Shutting down..")
}
