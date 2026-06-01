package config

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

// TestSaveAtomicConcurrentRace —  regression. Save must use a
// temp-file + rename idiom so a concurrent reader never observes a
// partially-written file. The write payload is large enough that the
// non-atomic O_TRUNC pattern leaves a measurable window of a truncated
// file on disk; a concurrent Load picks up that window and fails.
func TestSaveAtomicConcurrentRace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	// Build a fat config — many profiles widens the encode window so
	// the race against a concurrent reader is observable. With the
	// non-atomic O_TRUNC pattern the reader sees a truncated file
	// somewhere in the loop. With temp+rename the file is either the
	// previous full version or the new full version, never partial.
	makeCfg := func(prefix string) *Config {
		profiles := make([]Profile, 200)
		for i := range profiles {
			profiles[i] = Profile{
				Name:    fmt.Sprintf("%s-%d", prefix, i),
				BaseURL: fmt.Sprintf("https://example-%s-%d.atlassian.net", prefix, i),
				Email:   fmt.Sprintf("%s-%d@example.com", prefix, i),
			}
		}
		return &Config{DefaultProfile: profiles[0].Name, Profiles: profiles}
	}

	// Seed the file once so Load has something to read from the start.
	if err := Save(path, makeCfg("seed")); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	const writers = 4
	const readers = 8
	const iters = 30

	var writerWG sync.WaitGroup
	var readerWG sync.WaitGroup
	stop := make(chan struct{})

	writerWG.Add(writers)
	for i := range writers {
		go func(id int) {
			defer writerWG.Done()
			for n := range iters {
				cfg := makeCfg(fmt.Sprintf("w%d-%d", id, n))
				if err := Save(path, cfg); err != nil {
					t.Errorf("writer %d iter %d: %v", id, n, err)
					return
				}
			}
		}(i)
	}

	readerWG.Add(readers)
	for i := range readers {
		go func(id int) {
			defer readerWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := Load(WithPath(path)); err != nil {
					t.Errorf("reader %d observed truncated file: %v", id, err)
					return
				}
			}
		}(i)
	}

	writerWG.Wait()
	close(stop)
	readerWG.Wait()

	// No leftover temp files in the directory.
	entries, err := filepath.Glob(filepath.Join(dir, ".atomic-*.tmp"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("temp files left behind after Save: %v", entries)
	}
}
