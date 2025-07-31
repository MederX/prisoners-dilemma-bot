package models

import (
	"fmt"
	"sync"
	"time"
)

type GameState int

const (
	StateWaitingForPlayerB GameState = iota
	StateInProgress
	StateFinished
	StateWaitingRematch
)

type PlayerChoice string

const (
	ChoiceNone      PlayerChoice = ""
	ChoiceNegotiate PlayerChoice = "договориться"
	ChoiceDefect    PlayerChoice = "предать"
)

type RoundResult struct {
	Round         int
	PlayerAChoice PlayerChoice
	PlayerBChoice PlayerChoice
	PlayerAScore  int
	PlayerBScore  int
	Timestamp     time.Time
}

type Player struct {
	ID            int64
	Username      string
	Score         int
	CurrentChoice PlayerChoice
	LastMoveTime  time.Time
	WantsRematch  bool
}

// Session represents a single game instance between two players.
type Session struct {
	ID           int64
	PlayerA      *Player
	PlayerB      *Player
	TotalRounds  int
	CurrentRound int
	State        GameState
	Mutex        sync.Mutex
	History      []RoundResult
	TurnDeadline time.Time
}

// NEW: Helper method to get round history summary for a player
func (s *Session) GetHistorySummary(playerID int64) string {
	if len(s.History) == 0 {
		return "No previous rounds."
	}

	summary := "📚 Previous rounds:\n"
	for _, round := range s.History {
		var yourChoice, theirChoice PlayerChoice
		if playerID == s.PlayerA.ID {
			yourChoice = round.PlayerAChoice
			theirChoice = round.PlayerBChoice
		} else {
			yourChoice = round.PlayerBChoice
			theirChoice = round.PlayerAChoice
		}

		yourEmoji := "😇"
		if yourChoice == ChoiceDefect {
			yourEmoji = "😈"
		}

		theirEmoji := "😇"
		if theirChoice == ChoiceDefect {
			theirEmoji = "😈"
		}

		summary += fmt.Sprintf("R%d: ты %s, Он(а) %s\n", round.Round, yourEmoji, theirEmoji)
	}

	return summary
}

type PendingInvite struct {
	InviteID        string
	InviterID       int64
	InviterUsername string
	Rounds          int
}
