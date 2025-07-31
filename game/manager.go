package game

import (
	"fmt"
	"prisoners-dilemma-bot/models"
	"prisoners-dilemma-bot/utils"
	"sync"
	"time"
)

// Manager handles all active game sessions and pending invitations.
type Manager struct {
	sessions        map[int64]*models.Session
	pendingByID     map[string]*models.PendingInvite
	playerToSession map[int64]int64
	mu              sync.RWMutex
	timerCallbacks  map[int64]func()
}

// NewManager creates a new game manager.
func NewManager() *Manager {
	return &Manager{
		sessions:        make(map[int64]*models.Session),
		pendingByID:     make(map[string]*models.PendingInvite),
		playerToSession: make(map[int64]int64),
		timerCallbacks:  make(map[int64]func()),
	}
}

// CreateInvite creates a pending invitation and returns the invite ID.
func (m *Manager) CreateInvite(inviterID int64, inviterUsername string, rounds int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate a unique invite ID
	inviteID, err := utils.GenerateID(8)
	if err != nil {
		return "", fmt.Errorf("не удалось сгенерировать ID приглашения: %v", err)
	}

	invite := &models.PendingInvite{
		InviteID:        inviteID,
		InviterID:       inviterID,
		InviterUsername: inviterUsername,
		Rounds:          rounds,
	}

	m.pendingByID[inviteID] = invite
	return inviteID, nil
}

// AcceptInvite checks for a pending invite and creates a new game session if one exists.
func (m *Manager) AcceptInvite(inviteID string, accepterID int64, accepterUsername string) (*models.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	invite, ok := m.pendingByID[inviteID]
	if !ok {
		return nil, fmt.Errorf("это приглашение недействительно или истекло")
	}

	if invite.InviterID == accepterID {
		return nil, fmt.Errorf("вы не можете принять собственное приглашение")
	}

	session := &models.Session{
		ID: invite.InviterID,
		PlayerA: &models.Player{
			ID:       invite.InviterID,
			Username: invite.InviterUsername,
		},
		PlayerB: &models.Player{
			ID:       accepterID,
			Username: accepterUsername,
		},
		TotalRounds:  invite.Rounds,
		CurrentRound: 1,
		State:        models.StateInProgress,
		History:      make([]models.RoundResult, 0),
		TurnDeadline: time.Now().Add(2 * time.Minute),
	}

	m.sessions[session.ID] = session
	m.playerToSession[session.PlayerA.ID] = session.ID
	m.playerToSession[session.PlayerB.ID] = session.ID

	delete(m.pendingByID, inviteID)
	return session, nil
}

func (m *Manager) ForfeitGame(playerID int64) (*models.Session, *models.Player, error) {
	session, ok := m.FindSessionByPlayerID(playerID)
	if !ok {
		return nil, nil, fmt.Errorf("вы не находитесь в активной игре")
	}

	session.Mutex.Lock()
	defer session.Mutex.Unlock()

	if session.State != models.StateInProgress {
		return nil, nil, fmt.Errorf("эта игра уже завершена")
	}

	var winner *models.Player
	if playerID == session.PlayerA.ID {
		winner = session.PlayerB
	} else {
		winner = session.PlayerA
	}

	session.State = models.StateFinished
	m.endGame(session.ID)

	return session, winner, nil
}

// FindSessionByPlayerID retrieves the session a player is currently in.
func (m *Manager) FindSessionByPlayerID(playerID int64) (*models.Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessionID, ok := m.playerToSession[playerID]
	if !ok {
		return nil, false
	}
	session, ok := m.sessions[sessionID]
	return session, ok
}

// RecordChoice records a player's move for the current round.
// It returns true if both players have now made their choice for the round.
func (m *Manager) RecordChoice(playerID int64, choice models.PlayerChoice) (*models.Session, bool, error) {
	session, ok := m.FindSessionByPlayerID(playerID)
	if !ok {
		return nil, false, fmt.Errorf("активная игра не найдена")
	}

	session.Mutex.Lock()
	defer session.Mutex.Unlock()

	if session.State != models.StateInProgress {
		return nil, false, fmt.Errorf("игра не в процессе")
	}

	player := session.PlayerA
	if playerID == session.PlayerB.ID {
		player = session.PlayerB
	}

	player.CurrentChoice = choice
	player.LastMoveTime = time.Now()

	bothPlayersChose := session.PlayerA.CurrentChoice != models.ChoiceNone && session.PlayerB.CurrentChoice != models.ChoiceNone

	if bothPlayersChose {
		m.clearTimer(session.ID)
	}

	return session, bothPlayersChose, nil
}

func (m *Manager) ProcessRound(session *models.Session) (string, string) {
	session.Mutex.Lock()
	defer session.Mutex.Unlock()

	pA := session.PlayerA
	pB := session.PlayerB
	choiceA := pA.CurrentChoice
	choiceB := pB.CurrentChoice

	var resultA, resultB string
	var scoreA, scoreB int

	switch {
	case choiceA == models.ChoiceNegotiate && choiceB == models.ChoiceNegotiate:
		scoreA, scoreB = 3, 3
		resultA = "Вы оба выбрали сотрудничество 🤝."
		resultB = resultA
	case choiceA == models.ChoiceNegotiate && choiceB == models.ChoiceDefect:
		scoreA, scoreB = 0, 5
		resultA = fmt.Sprintf("Вы сотрудничали 😇, но %s предал 😈.", pB.Username)
		resultB = fmt.Sprintf("%s сотрудничал 😇, но вы предали 😈.", pA.Username)
	case choiceA == models.ChoiceDefect && choiceB == models.ChoiceNegotiate:
		scoreA, scoreB = 5, 0
		resultA = fmt.Sprintf("Вы предали 😈, пока %s сотрудничал 😇.", pB.Username)
		resultB = fmt.Sprintf("Вы сотрудничали 😇, но %s предал 😈.", pA.Username)
	case choiceA == models.ChoiceDefect && choiceB == models.ChoiceDefect:
		scoreA, scoreB = 1, 1
		resultA = "Вы оба выбрали предательство ⚔️."
		resultB = resultA
	}

	pA.Score += scoreA
	pB.Score += scoreB

	// NEW: Store round result in history
	roundResult := models.RoundResult{
		Round:         session.CurrentRound,
		PlayerAChoice: choiceA,
		PlayerBChoice: choiceB,
		PlayerAScore:  scoreA,
		PlayerBScore:  scoreB,
		Timestamp:     time.Now(),
	}
	session.History = append(session.History, roundResult)

	roundSummaryA := fmt.Sprintf("Вы получили %d очков. Соперник получил %d очков.", scoreA, scoreB)
	roundSummaryB := fmt.Sprintf("Вы получили %d очков. Соперник получил %d очков.", scoreB, scoreA)

	resultMsgA := resultA + "\n" + roundSummaryA
	resultMsgB := resultB + "\n" + roundSummaryB

	// Prepare for next round or end game
	session.CurrentRound++
	pA.CurrentChoice = models.ChoiceNone
	pB.CurrentChoice = models.ChoiceNone

	if session.CurrentRound > session.TotalRounds {
		session.State = models.StateFinished
	} else {
		session.TurnDeadline = time.Now().Add(2 * time.Minute)
	}

	return resultMsgA, resultMsgB
}

func (m *Manager) SetRematchPreference(playerID int64, wantsRematch bool) (*models.Session, bool, error) {
	m.mu.RLock()
	sessionID, hasSession := m.playerToSession[playerID]
	m.mu.RUnlock()

	if !hasSession {
		return nil, false, fmt.Errorf("вы не находитесь в игре")
	}

	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()

	if !ok {
		return nil, false, fmt.Errorf("сессия не найдена")
	}

	session.Mutex.Lock()
	defer session.Mutex.Unlock()

	if session.State != models.StateFinished {
		return nil, false, fmt.Errorf("игра еще не завершена")
	}

	if playerID == session.PlayerA.ID {
		session.PlayerA.WantsRematch = wantsRematch
	} else {
		session.PlayerB.WantsRematch = wantsRematch
	}

	if !wantsRematch {
		m.ClearPlayerSession(session.PlayerA.ID)
		m.ClearPlayerSession(session.PlayerB.ID)
		return session, false, nil
	}

	bothWantRematch := session.PlayerA.WantsRematch && session.PlayerB.WantsRematch

	return session, bothWantRematch, nil
}

func (m *Manager) ClearPlayerSession(playerID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.playerToSession, playerID)
}

func (m *Manager) StartRematch(sessionID int64) (*models.Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldSession, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("исходная сессия не найдена")
	}

	newSession := &models.Session{
		ID: oldSession.ID,
		PlayerA: &models.Player{
			ID:            oldSession.PlayerA.ID,
			Username:      oldSession.PlayerA.Username,
			Score:         0,
			CurrentChoice: models.ChoiceNone,
			WantsRematch:  false,
		},
		PlayerB: &models.Player{
			ID:            oldSession.PlayerB.ID,
			Username:      oldSession.PlayerB.Username,
			Score:         0,
			CurrentChoice: models.ChoiceNone,
			WantsRematch:  false,
		},
		TotalRounds:  oldSession.TotalRounds,
		CurrentRound: 1,
		State:        models.StateInProgress,
		History:      make([]models.RoundResult, 0),
		TurnDeadline: time.Now().Add(2 * time.Minute),
	}

	m.sessions[sessionID] = newSession
	m.playerToSession[newSession.PlayerA.ID] = sessionID
	m.playerToSession[newSession.PlayerB.ID] = sessionID

	return newSession, nil
}

func (m *Manager) SetTurnTimer(sessionID int64, onTimeout func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if callback, exists := m.timerCallbacks[sessionID]; exists {

		_ = callback
	}
	m.timerCallbacks[sessionID] = onTimeout
	go func() {
		time.Sleep(2 * time.Minute)
		m.mu.RLock()
		callback, exists := m.timerCallbacks[sessionID]
		m.mu.RUnlock()

		if exists && callback != nil {
			callback()
		}
	}()
}

func (m *Manager) clearTimer(sessionID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.timerCallbacks, sessionID)
}

func (m *Manager) HandleTimeout(sessionID int64) (*models.Session, *models.Player, error) {
	session, ok := m.FindSessionByPlayerID(sessionID)
	if !ok {
		return nil, nil, fmt.Errorf("сессия не найдена")
	}

	session.Mutex.Lock()
	defer session.Mutex.Unlock()

	if session.State != models.StateInProgress {
		return nil, nil, fmt.Errorf("игра не в процессе")
	}

	var timeoutPlayer, activePlayer *models.Player
	if session.PlayerA.CurrentChoice == models.ChoiceNone {
		timeoutPlayer = session.PlayerA
		activePlayer = session.PlayerB
	} else {
		timeoutPlayer = session.PlayerB
		activePlayer = session.PlayerA
	}

	timeoutPlayer.CurrentChoice = models.ChoiceDefect

	if activePlayer.CurrentChoice != models.ChoiceNone {
		m.ProcessRound(session)
	}

	session.State = models.StateFinished
	m.clearTimer(sessionID)

	return session, activePlayer, nil
}

func (m *Manager) endGame(sessionID int64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, ok := m.sessions[sessionID]
	if !ok {
		return
	}

	m.clearTimer(sessionID)

	go func() {
		time.Sleep(5 * time.Minute)
		m.mu.Lock()
		defer m.mu.Unlock()
		if currentSession, exists := m.sessions[sessionID]; exists && currentSession.State == models.StateFinished {
			delete(m.playerToSession, currentSession.PlayerA.ID)
			delete(m.playerToSession, currentSession.PlayerB.ID)
			delete(m.sessions, sessionID)
		}
	}()
}
