package session

import (
	"io"
	"sync"
	"time"
)

type SessionStatus string

const (
	StatusActive   SessionStatus = "Active"
	StatusInactive SessionStatus = "Inactive"
	StatusPending  SessionStatus = "Pending"
)

type Session struct {
	ID           int
	Type         string
	TargetIP     string
	Port         int
	Status       SessionStatus
	Created      time.Time
	LastActivity time.Time
	RW           io.ReadWriteCloser
}

type SessionManager struct {
	sessions map[int]*Session
	nextID   int
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[int]*Session),
		nextID:   1,
	}
}

func (sm *SessionManager) CreateSession(sType string, port int, target string, rw io.ReadWriteCloser) int {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	id := sm.nextID
	sm.nextID++

	sm.sessions[id] = &Session{
		ID:           id,
		Type:         sType,
		TargetIP:     target,
		Port:         port,
		Status:       StatusActive,
		Created:      time.Now(),
		LastActivity: time.Now(),
		RW:           rw,
	}

	return id
}

func (sm *SessionManager) CloseSession(id int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if s, ok := sm.sessions[id]; ok {
		if s.RW != nil {
			s.RW.Close()
		}
		s.Status = StatusInactive
	}
}

func (sm *SessionManager) ListSessions() []*Session {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	list := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		list = append(list, s)
	}
	return list
}

func (sm *SessionManager) GetSession(id int) (*Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	s, ok := sm.sessions[id]
	return s, ok
}
