package toolbox

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/intox/shelly/pkg/config"
)

type Toolbox struct {
	Dir string
	Cfg *config.Config
}

func NewToolbox(dir string, cfg *config.Config) *Toolbox {
	return &Toolbox{
		Dir: dir,
		Cfg: cfg,
	}
}

func (t *Toolbox) EnsureDir() error {
	return os.MkdirAll(t.Dir, 0755)
}

func (t *Toolbox) DownloadAll() []error {
	var errs []error
	for name, item := range t.Cfg.Toolbox {
		for arch, details := range item {
			fmt.Printf("Downloading %s (%s)...\n", name, arch)
			err := t.DownloadFile(details.Download, details.Filename)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to download %s/%s: %w", name, arch, err))
			}
		}
	}
	return errs
}

func (t *Toolbox) DownloadFile(url, filename string) error {
	destPath := filepath.Join(t.Dir, filename)

	// Create the file
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func (t *Toolbox) List() ([]string, error) {
	files, err := os.ReadDir(t.Dir)
	if err != nil {
		return nil, err
	}

	var filenames []string
	for _, f := range files {
		if !f.IsDir() {
			filenames = append(filenames, f.Name())
		}
	}
	return filenames, nil
}
