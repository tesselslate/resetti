package reset_test

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/reset"
)

var enabledConf = cfg.Profile{
	General: struct {
		ResetType   string "toml:\"type\""
		CountResets bool   "toml:\"count_resets\""
		CountPath   string "toml:\"resets_file\""
	}{
		CountResets: true,
	},
}

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
	time.Sleep(time.Millisecond * 5)
	cancel()
	wg.Wait()
}

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

func TestCounterDisabled(t *testing.T) {
	makeCounter(t, cfg.Profile{})
}

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
