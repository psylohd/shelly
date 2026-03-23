package shell

import (
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/intox/shelly/pkg/session"
)

type Listener interface {
	Start(onConnect func(int)) error
	Stop() error
}

type NetcatListener struct {
	Host     string
	Port     int
	SM       *session.SessionManager
	ln       net.Listener
	sessions []int
	mu       sync.Mutex
	stop     chan struct{}
}

func NewNetcatListener(host string, port int, sm *session.SessionManager) *NetcatListener {
	return &NetcatListener{
		Host: host,
		Port: port,
		SM:   sm,
		stop: make(chan struct{}),
	}
}

func (l *NetcatListener) Start(onConnect func(int)) error {
	addr := fmt.Sprintf("%s:%d", l.Host, l.Port)
	ln, err := net.Listen("tcp4", addr)
	if err != nil {
		return err
	}
	l.ln = ln
	log.Printf("[LISTENER] Listening on %s", addr)

	go func() {
		for {
			conn, err := l.ln.Accept()
			if err != nil {
				select {
				case <-l.stop:
					return
				default:
					continue
				}
			}

			// Create a session
			remoteAddr := conn.RemoteAddr().String()
			id := l.SM.CreateSession("netcat", l.Port, remoteAddr, conn)
			log.Printf("[LISTENER] New session %d from %s", id, remoteAddr)

			l.mu.Lock()
			l.sessions = append(l.sessions, id)
			l.mu.Unlock()

			if onConnect != nil {
				onConnect(id)
			}
		}
	}()

	return nil
}

func (l *NetcatListener) Stop() error {
	close(l.stop)
	if l.ln != nil {
		return l.ln.Close()
	}
	return nil
}
