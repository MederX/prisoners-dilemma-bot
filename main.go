package main

import (
	"log"
	"os"
	"prisoners-dilemma-bot/bot"
	"prisoners-dilemma-bot/game"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable not set")
	}

	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	api.Debug = false

	log.Printf("Authorized on account %s", api.Self.UserName)

	gameManager := game.NewManager()
	telegramBot := bot.NewBot(api, gameManager)

	telegramBot.Start()
}
