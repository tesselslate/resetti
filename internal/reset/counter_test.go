package reset_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/reset"
)

// enabledConf contains a configuration profile with `count_resets` enabled.
var enabledConf = cfg.Profile{
	General: struct {
		ResetType   string "toml:\"type\""
		CountResets bool   "toml:\"count_resets\""
		CountPath   string "toml:\"resets_file\""
	}{
		CountResets: true,
	},
}

// makeCounter creates a new Counter with the given configuration and increments
// it 1000 times.
func makeCounter(t *testing.T, conf cfg.Profile) {
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(context.Background())
	counter, err := reset.NewCounter(ctx, &wg, conf)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i += 1 {
		go func() {
			counter.Increment()
		}()
	}
	cancel()
	wg.Wait()
}

// TestCounterCreate creates a fresh counter and checks that it captures all
// 1000 resets.
func TestCounterCreate(t *testing.T) {
	path := t.TempDir() + "/count"
	conf := enabledConf
	conf.General.CountPath = path
	makeCounter(t, conf)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	num, err := strconv.Atoi(string(strings.TrimSpace(string(content))))
	if err != nil {
		t.Fatal(err)
	}
	if num != 1000 {
		t.Fatalf("got %d, want 1000", num)
	}
}

// TestCounterDisabled tests that the reset counter does not crash when
// disabled.
func TestCounterDisabled(t *testing.T) {
	makeCounter(t, cfg.Profile{})
}

// TestCounterRead creates a counter with 1000 existing resets and checks that
// it ends up with 2000 resets.
func TestCounterRead(t *testing.T) {
	path := t.TempDir() + "/count"
	conf := enabledConf
	conf.General.CountPath = path
	err := os.WriteFile(path, []byte("1000"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	makeCounter(t, conf)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	num, err := strconv.Atoi(string(strings.TrimSpace(string(content))))
	if err != nil {
		t.Fatal(err)
	}
	if num != 2000 {
		t.Fatalf("got %d, want 2000", num)
	}
}
