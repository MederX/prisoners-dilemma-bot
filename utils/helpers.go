// utils/helpers.go
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"prisoners-dilemma-bot/models"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ChoiceKeyboard creates the inline keyboard for players to make their move.
func ChoiceKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🤝 договориться", string(models.ChoiceNegotiate)),
			tgbotapi.NewInlineKeyboardButtonData("⚔️ предать", string(models.ChoiceDefect)),
		),
	)
}

// MainMenuKeyboard creates the persistent keyboard for the main menu.
func MainMenuKeyboard() tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("🚀 Создать новую игру"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("❓ Помощь"),
		),
	)
}

// RoundsKeyboard creates the inline keyboard for selecting the number of rounds.
func RoundsKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("10 Раундов", "rounds_10"),
			tgbotapi.NewInlineKeyboardButtonData("15 Раундов", "rounds_15"),
			tgbotapi.NewInlineKeyboardButtonData("20 Раундов", "rounds_20"),
		),
	)
}

// NEW: RematchKeyboard creates the inline keyboard for rematch options
func RematchKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Играть снова", "rematch_yes"),
			tgbotapi.NewInlineKeyboardButtonData("🚪 Главное меню", "rematch_no"),
		),
	)
}

// GenerateID creates a random, URL-safe string for invite links.
func GenerateID(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
