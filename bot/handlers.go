// bot/handlers.go
package bot

import (
	"fmt"
	"log"
	"prisoners-dilemma-bot/game"
	"prisoners-dilemma-bot/models"
	"prisoners-dilemma-bot/utils"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Bot struct {
	api     *tgbotapi.BotAPI
	manager *game.Manager
}

func NewBot(api *tgbotapi.BotAPI, manager *game.Manager) *Bot {
	return &Bot{api: api, manager: manager}
}

func (b *Bot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.api.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			b.handleMessage(update.Message)
		} else if update.CallbackQuery != nil {
			b.handleCallbackQuery(update.CallbackQuery)
		}
	}
}

// handleMessage routes incoming text messages to the correct handler.
func (b *Bot) handleMessage(message *tgbotapi.Message) {
	// Route non-command text from main menu keyboard
	if !message.IsCommand() {
		switch message.Text {
		case "🚀 Создать новую игру":
			b.handleNewGame(message)
			return
		case "❓ Помощь":
			b.handleHelp(message.Chat.ID)
			return
		}
	}

	switch message.Command() {
	case "start":
		b.handleStart(message)
	case "help":
		b.handleHelp(message.Chat.ID)
	case "quit":
		b.handleQuit(message)
	default:
		b.reply(message.Chat.ID, "🤔 Неизвестная команда. Используйте меню ниже или введите /help.", false, nil)
	}
}

func (b *Bot) handleStart(message *tgbotapi.Message) {
	payload := message.CommandArguments()
	if strings.HasPrefix(payload, "invite_") {
		inviteID := strings.TrimPrefix(payload, "invite_")
		b.handleAccept(inviteID, message)
		return
	}

	// Standard start
	msgText := "Добро пожаловать в бот \"Дилемма Заключенного\"!" +
		"Используйте меню ниже, чтобы начать игру или изучить правила."
	keyboard := utils.MainMenuKeyboard()
	b.reply(message.Chat.ID, msgText, false, &keyboard)
}

func (b *Bot) handleHelp(chatID int64) {
	helpText := "📜 Правила игры 📜\n\n" +
		"Это игра на стратегию и доверие для двух игроков.\n\n" +
		"Геймплей:\n" +
		"1. Один игрок создает игру и отправляет ссылку-приглашение.\n" +
		"2. В каждом раунде вы тайно выбираете: Сотрудничать или Предать.\n\n" +
		"Подсчет очков:\n" +
		"• Если оба Сотрудничают: +3 очка каждому 🤝\n" +
		"• Если вы Предаете, а соперник Сотрудничает: +5 очков вам, 0 сопернику 😈\n" +
		"• Если оба Предают: +1 очко каждому ⚔️\n\n" +
		"Цель - набрать максимальное количество очков после всех раундов. Будете ли вы сотрудничать для взаимной выгоды или предавать ради личного преимущества?"
	b.reply(chatID, helpText, false, nil)
}

func (b *Bot) handleNewGame(message *tgbotapi.Message) {
	if _, inGame := b.manager.FindSessionByPlayerID(message.From.ID); inGame {
		b.reply(message.Chat.ID, "Вы уже в игре! Введите /quit, чтобы покинуть текущую игру.", false, nil)
		return
	}

	msgText := "Сколько раундов вы хотите играть?"
	keyboard := utils.RoundsKeyboard()
	msg := tgbotapi.NewMessage(message.Chat.ID, msgText)
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) handleQuit(message *tgbotapi.Message) {
	_, winner, err := b.manager.ForfeitGame(message.From.ID)
	if err != nil {
		b.reply(message.From.ID, err.Error(), false, nil)
		return
	}

	quitter := message.From

	b.reply(quitter.ID, "Вы покинули игру.", false, nil)
	b.reply(winner.ID, fmt.Sprintf("😢 %s покинул игру. Вы побеждаете по умолчанию!", quitter.UserName), false, nil)
}

func (b *Bot) handleAccept(inviteID string, message *tgbotapi.Message) {
	accepterID := message.From.ID
	accepterUsername := message.From.UserName

	if _, inGame := b.manager.FindSessionByPlayerID(accepterID); inGame {
		b.reply(message.Chat.ID, "Вы уже в игре! Вы не можете принять другое приглашение.", false, nil)
		return
	}

	session, err := b.manager.AcceptInvite(inviteID, accepterID, accepterUsername)
	if err != nil {
		b.reply(message.Chat.ID, err.Error(), false, nil)
		return
	}

	// Notify both players and start the game - using simple text without usernames first
	msgToInviter := fmt.Sprintf("🎉 Ваше приглашение принято! Игра начинается сейчас.")
	b.reply(session.PlayerA.ID, msgToInviter, false, nil)

	msgToAccepter := fmt.Sprintf("✅ Вы присоединились к игре! Игра начинается сейчас.")
	b.reply(session.PlayerB.ID, msgToAccepter, false, nil)

	b.promptNextRound(session)
}

func (b *Bot) handleCallbackQuery(cb *tgbotapi.CallbackQuery) {
	b.api.Send(tgbotapi.NewCallback(cb.ID, ""))

	data := cb.Data

	if strings.HasPrefix(data, "rounds_") {
		b.handleRoundSelection(cb)
	} else if data == string(models.ChoiceNegotiate) || data == string(models.ChoiceDefect) {
		b.handleGameChoice(cb)
	} else if strings.HasPrefix(data, "rematch_") {
		b.handleRematchChoice(cb)
	}
}

func (b *Bot) handleGameChoice(cb *tgbotapi.CallbackQuery) {
	playerID := cb.From.ID
	choice := models.PlayerChoice(cb.Data)

	session, bothChose, err := b.manager.RecordChoice(playerID, choice)
	if err != nil {
		// This can happen if a player clicks an old button after a game ends
		log.Printf("Error recording choice for player %d: %v", playerID, err)
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Эта игра больше не активна.")
		b.api.Send(editMsg)
		return
	}

	var choiceText string
	if choice == models.ChoiceNegotiate {
		choiceText = "Сотрудничать"
	} else {
		choiceText = "Предать"
	}
	chosenText := fmt.Sprintf("Вы выбрали: %s. Ожидаем другого игрока...", choiceText)
	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, chosenText)
	b.api.Send(editMsg)

	if bothChose {
		resultA, resultB := b.manager.ProcessRound(session)

		scoreUpdateA := fmt.Sprintf("\n\nСчет:\n- Вы: %d\n- %s: %d", session.PlayerA.Score, session.PlayerB.Username, session.PlayerB.Score)
		b.reply(session.PlayerA.ID, resultA+scoreUpdateA, false, nil)

		scoreUpdateB := fmt.Sprintf("\n\nСчет:\n- Вы: %d\n- %s: %d", session.PlayerB.Score, session.PlayerA.Username, session.PlayerA.Score)
		b.reply(session.PlayerB.ID, resultB+scoreUpdateB, false, nil)

		if session.State == models.StateFinished {
			b.announceWinner(session)
		} else {
			b.promptNextRound(session)
			// NEW: Set up turn timer
			b.setupTurnTimer(session)
		}
	}
}

func (b *Bot) handleRoundSelection(cb *tgbotapi.CallbackQuery) {
	roundStr := strings.TrimPrefix(cb.Data, "rounds_")
	rounds, _ := strconv.Atoi(roundStr)

	inviterID := cb.From.ID
	inviterUsername := cb.From.UserName

	inviteID, err := b.manager.CreateInvite(inviterID, inviterUsername, rounds)
	if err != nil {
		log.Printf("Error creating invite: %v", err)
		b.reply(inviterID, "Извините, произошла ошибка при создании игры. Попробуйте еще раз.", false, nil)
		return
	}

	botUsername := b.api.Self.UserName
	inviteURL := fmt.Sprintf("https://t.me/%s?start=invite_%s", botUsername, inviteID)

	var roundsText string
	switch rounds {
	case 1:
		roundsText = "1 раунд"
	case 2, 3, 4:
		roundsText = fmt.Sprintf("%d раунда", rounds)
	default:
		roundsText = fmt.Sprintf("%d раундов", rounds)
	}

	msgText := fmt.Sprintf(
		"✅ Ваша игра на %s готова!\n\n"+
			"Поделитесь этим приглашением с другим игроком.\n"+
			"Вы можете переслать это сообщение или скопировать ссылку.", roundsText)

	// Create a button with the invite link
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonURL("➡️ Принять приглашение", inviteURL),
		),
	)

	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, msgText)
	editMsg.ReplyMarkup = &keyboard
	b.api.Send(editMsg)
}

func (b *Bot) promptNextRound(session *models.Session) {
	promptText := fmt.Sprintf("Раунд %d из %d\nВаш ход?", session.CurrentRound, session.TotalRounds)
	keyboard := utils.ChoiceKeyboard()

	msgA := tgbotapi.NewMessage(session.PlayerA.ID, promptText)
	msgA.ReplyMarkup = keyboard
	b.api.Send(msgA)

	msgB := tgbotapi.NewMessage(session.PlayerB.ID, promptText)
	msgB.ReplyMarkup = keyboard
	b.api.Send(msgB)
}

func (b *Bot) announceWinner(session *models.Session) {
	var winnerText string
	pA := session.PlayerA
	pB := session.PlayerB

	switch {
	case pA.Score > pB.Score:
		winnerText = fmt.Sprintf("🏆 %s победил! 🏆", pA.Username)
	case pB.Score > pA.Score:
		winnerText = fmt.Sprintf("🏆 %s победил! 🏆", pB.Username)
	default:
		winnerText = "🤝 Ничья! 🤝"
	}

	finalMsg := fmt.Sprintf(
		"🏁 Игра окончена! 🏁\n\n"+
			"Итоговый счет:\n"+
			"---------------------\n"+
			"Игрок: %s\nОчки: %d\n\n"+
			"Игрок: %s\nОчки: %d\n"+
			"---------------------\n\n"+
			"%s",
		pA.Username, pA.Score, pB.Username, pB.Score, winnerText)
	b.reply(pA.ID, finalMsg, false, nil)
	b.reply(pB.ID, finalMsg, false, nil)

	// Ask players if they want a rematch
	rematchKeyboard := utils.RematchKeyboard()
	b.reply(pA.ID, "Хотите реванш?", false, rematchKeyboard)
	b.reply(pB.ID, "Хотите реванш?", false, rematchKeyboard)
}

// handleRematchChoice processes a player's rematch choice
func (b *Bot) handleRematchChoice(cb *tgbotapi.CallbackQuery) {
	wantsRematch := strings.TrimPrefix(cb.Data, "rematch_") == "yes"
	playerID := cb.From.ID

	session, bothWantRematch, err := b.manager.SetRematchPreference(playerID, wantsRematch)
	if err != nil {
		b.reply(playerID, err.Error(), false, nil)
		return
	}

	var otherPlayer *models.Player
	if playerID == session.PlayerA.ID {
		otherPlayer = session.PlayerB
	} else {
		otherPlayer = session.PlayerA
	}

	keyboard := utils.MainMenuKeyboard()
	welcomeText := "Добро пожаловать в бот \"Дилемма Заключенного\"!\n\nИспользуйте меню ниже, чтобы начать новую игру или изучить правила."

	if !wantsRematch {
		// This player chose "Main Menu"
		editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Возвращаемся в главное меню...")
		b.api.Send(editMsg)

		b.reply(playerID, welcomeText, false, &keyboard)
		b.reply(otherPlayer.ID, "Другой игрок не захотел играть реванш. Возвращаемся в главное меню...", false, nil)
		b.reply(otherPlayer.ID, welcomeText, false, &keyboard)
		return
	}

	// This player wants a rematch
	editMsg := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, "Вы хотите реванш! Ждем другого игрока...")
	b.api.Send(editMsg)

	if bothWantRematch {
		// Start the rematch
		newSession, err := b.manager.StartRematch(session.ID)
		if err != nil {
			b.reply(session.PlayerA.ID, "Не удалось начать реванш: "+err.Error(), false, nil)
			b.reply(session.PlayerB.ID, "Не удалось начать реванш: "+err.Error(), false, nil)
			return
		}

		// Notify both players
		b.reply(newSession.PlayerA.ID, "🎮 Реванш начинается!", false, nil)
		b.reply(newSession.PlayerB.ID, "🎮 Реванш начинается!", false, nil)

		b.promptNextRound(newSession)
		b.setupTurnTimer(newSession)
	}
}

func (b *Bot) setupTurnTimer(session *models.Session) {
	b.manager.SetTurnTimer(session.ID, func() {
		session, winner, err := b.manager.HandleTimeout(session.ID)
		if err != nil {
			return // Session already ended or other error
		}

		// Notify players of timeout
		timeoutMsg := fmt.Sprintf("⏰ Время вышло! %s слишком долго не делал ход.", winner.Username)
		b.reply(session.PlayerA.ID, timeoutMsg, false, nil)
		b.reply(session.PlayerB.ID, timeoutMsg, false, nil)

		// If the game ended due to timeout, announce the winner
		if session.State == models.StateFinished {
			b.announceWinner(session)
		}
	})
}

func (b *Bot) reply(chatID int64, text string, markdown bool, keyboard interface{}) {
	msg := tgbotapi.NewMessage(chatID, text)
	if markdown {
		msg.ParseMode = "Markdown"
	}
	if keyboard != nil {
		msg.ReplyMarkup = keyboard
	}
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Failed to send message to %d: %v", chatID, err)
	}
}
