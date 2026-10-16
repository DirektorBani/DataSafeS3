package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNotificationJSONShareBlock(t *testing.T) {
	raw, err := NotificationJSON(Event{
		ID:        "e1",
		Type:      "ShareCreated",
		Bucket:    "photos",
		Key:       "a.txt",
		CreatedAt: time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC),
		Payload: map[string]any{
			"link_id": "lnk", "actor": "ada", "client_ip": "203.0.113.5",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Records []struct {
			EventName string `json:"eventName"`
		} `json:"Records"`
		DataSafe struct {
			Share struct {
				LinkID   string `json:"link_id"`
				Actor    string `json:"actor"`
				ClientIP string `json:"client_ip"`
			} `json:"share"`
		} `json:"datasafe"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Records[0].EventName != "datasafe:Share:Created" {
		t.Fatalf("eventName %s", msg.Records[0].EventName)
	}
	if msg.DataSafe.Share.LinkID != "lnk" || msg.DataSafe.Share.Actor != "ada" || msg.DataSafe.Share.ClientIP != "203.0.113.5" {
		t.Fatalf("share block %+v", msg.DataSafe.Share)
	}
	if strings.Contains(string(raw), "secret") {
		t.Fatal("unexpected secret")
	}
}
