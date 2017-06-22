package main

import "io/ioutil"

import "github.com/codecat/go-libs/log"
import "gopkg.in/yaml.v2"

type ScheduleItem struct {
	Time struct {
		Day int
		Hour int
		Minute int
	}

	Name string
	Color string
	Live bool
}

var schedule struct {
	Items []ScheduleItem
}

func readScheduleFile() []byte {
	scheduleData, err := ioutil.ReadFile(config.ScheduleFile)
	if err != nil {
		log.Fatal("Couldn't read schedule file: %s", err.Error())
		return nil
	}
	return []byte(scheduleData)
}

func loadSchedule() bool {
	scheduleData := readScheduleFile()
	if scheduleData == nil {
		return false
	}

	err := yaml.Unmarshal(scheduleData, &schedule)
	if err != nil {
		log.Fatal("Couldn't unmarshal yaml data: %s", err.Error())
		return false
	}

	log.Info("Schedule loaded")
	return true
}
