package hub

import (
    "encoding/json"
    "testing"

    "github.com/solarops/shared/models"
)

func TestHubRegisterUnregister(t *testing.T) {
    h := New()

    ch := make(chan []byte, 10)
    h.Register(ch)

    if len(h.clients) != 1 {
        t.Errorf("expected 1 client, got %d", len(h.clients))
    }

    h.Unregister(ch)
    if len(h.clients) != 0 {
        t.Errorf("expected 0 clients, got %d", len(h.clients))
    }
}

func TestHubBroadcast(t *testing.T) {
    h := New()

    ch1 := make(chan []byte, 10)
    ch2 := make(chan []byte, 10)
    h.Register(ch1)
    h.Register(ch2)

    msg := models.WSMessage{Type: models.MsgPlantData, Payload: "test"}
    data, _ := json.Marshal(msg)
    h.Broadcast(data)

    got1 := <-ch1
    got2 := <-ch2

    if string(got1) != string(data) || string(got2) != string(data) {
        t.Error("broadcast should deliver to all clients")
    }
}
