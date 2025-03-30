package main

import (
	"log"
	"os"
	"voting-bot/bot"
	"voting-bot/tarantool"
)

func main() {
	tarantoolAddr := os.Getenv("TARANTOOL_ADDRESS")
	tarantoolUser := os.Getenv("TARANTOOL_USER")
	tarantoolPass := os.Getenv("TARANTOOL_PASSWORD")

	tc, err := tarantool.NewTarantoolClient(tarantoolAddr, tarantoolUser, tarantoolPass)
	if err != nil {
		log.Fatalf("Failed to connect to Tarantool: %v", err)
	}

	mattermostURL := os.Getenv("MATTERMOST_URL")
	botToken := os.Getenv("MATTERMOST_TOKEN")

	votingBot, err := bot.NewBot(mattermostURL, botToken, tc)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	votingBot.Listen()
	select {}
}
