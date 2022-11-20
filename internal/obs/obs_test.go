package obs_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/woofdoggo/resetti/internal/obs"
)

func TestObsWebsocket(t *testing.T) {
	if _, set := os.LookupEnv("RESETTI_TEST_OBS"); !set {
		t.Skip()
	}
	client := obs.Client{}
	ctx, cancel := context.WithCancel(context.Background())
	_, err := client.Connect(ctx, "localhost:4440", "")
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			t.Skip()
		} else {
			t.Fatal(err)
		}
	}
	err = client.CreateSceneCollection("resetti-test")
	if err != nil {
		t.Fatal(err)
	}
	err = client.SetSceneCollection("resetti-test")
	if err != nil {
		t.Fatal(err)
	}
	_, active, err := client.GetSceneCollectionList()
	if err != nil {
		t.Fatal(err)
	}
	if active != "resetti-test" {
		t.Fatal(errors.New("scene collection is not set to resetti-test"))
	}
	err = client.CreateScene("test-scene")
	if err != nil {
		t.Fatal(err)
	}
	cancel()
}
