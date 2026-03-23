package serve

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

type Server struct {
	Port int
	Dir  string
	srv  *http.Server
}

func NewServer(port int, dir string) *Server {
	return &Server{
		Port: port,
		Dir:  dir,
	}
}

func (s *Server) Start() error {
	// Ensure the directory exists
	if err := os.MkdirAll(s.Dir, 0755); err != nil {
		return fmt.Errorf("failed to create serve directory: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir(s.Dir)))

	ln, err := net.Listen("tcp4", fmt.Sprintf(":%d", s.Port))
	if err != nil {
		return err
	}

	s.srv = &http.Server{
		Handler: mux,
	}

	go func() {
		if err := s.srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			fmt.Printf("HTTP server error: %v\n", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	if s.srv != nil {
		return s.srv.Close()
	}
	return nil
}

// AddFile copies a file from a source path to the serve directory
func (s *Server) AddFile(srcPath string) error {
	filename := filepath.Base(srcPath)
	destPath := filepath.Join(s.Dir, filename)

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	return os.WriteFile(destPath, data, 0644)
}
