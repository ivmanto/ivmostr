package services

import (
	"testing"

	gn "github.com/studiokaiji/go-nostr"
)

// write me a unit test for the functions in auxfun.go file
func Test_mapToEvent(t *testing.T) {
	tests := []struct {
		name    string
		m       map[string]interface{}
		want    *gn.Event
		wantErr bool
	}{
		// TODO: Add test cases.
		// write me an example for two use-cases
		// test case 1:
		{
			name: "test case 1",
			m: map[string]interface{}{
				"id":         "b22f429ac7222530b6111191c160cdf5730a482ab18177c380b138278e443afa",
				"pubkey":     "9f9c3e46933b9a33702abde78383745a1ebcd99f59a53ec9286beee72a1072de",
				"created_at": 1697743946,
				"kind":       1,
				"tags":       []interface{}{`["e","","wss://nostr.ivmanto.dev"]`},
				"content":    "Test event by ivmanto",
				"sig":        "77d4065b1dda7320f3257083efecbc2db6265f3b2c5b28607ffd31acb3d0f5f1a272f81d9482cd56c96368cb0c8f1a840845e9adb13bb295b639040ae91333e1",
			},
			want: &gn.Event{
				ID:        "b22f429ac7222530b6111191c160cdf5730a482ab18177c380b138278e443afa",
				PubKey:    "9f9c3e46933b9a33702abde78383745a1ebcd99f59a53ec9286beee72a1072de",
				CreatedAt: 1697743946,
				Kind:      1,
				Tags:      gn.Tags{[]string{"e", "", "wss://nostr.ivmanto.dev"}},
				Content:   "Test event by ivmanto",
				Sig:       "77d4065b1dda7320f3257083efecbc2db6265f3b2c5b28607ffd31acb3d0f5f1a272f81d9482cd56c96368cb0c8f1a840845e9adb13bb295b639040ae91333e1",
			},
			wantErr: false,
		},
		// test case 2:
		{
			name: "test case 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mapToEvent(tt.m)
			if (err != nil) != tt.wantErr {
				t.Errorf("mapToEvent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("mapToEvent() got = %v, want %v", got, tt.want)
			}
		})
	}
}
