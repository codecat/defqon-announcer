package main

import "io/ioutil"

import "github.com/codecat/go-libs/log"
import "gopkg.in/yaml.v2"

var config struct {
	Discord struct {
		Token string
		Admins []string

		Announce struct {
			Channel string
		}

		Voice struct {
			Guild string
			Channel string
			Source string
		}
	}
	ScheduleFile string
}

func readConfigFile() []byte {
	configData, err := ioutil.ReadFile("config.yaml")
	if err != nil {
		log.Fatal("Couldn't read config file: %s", err.Error())
		return nil
	}
	return []byte(configData)
}

func loadConfig() bool {
	configData := readConfigFile()
	if configData == nil {
		return false
	}

	err := yaml.Unmarshal(configData, &config)
	if err != nil {
		log.Fatal("Couldn't unmarshal yaml data: %s", err.Error())
		return false
	}

	log.Info("Config loaded")
	return true
}
