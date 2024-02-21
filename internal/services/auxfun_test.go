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
				"id":        "1234567890123456789012345678901234567890",
				"pubkey":    "1234567890123456789012345678901234567890",
				"created_at": 1662612183,
				"kind":      1,
				"tags":      []interface{}{"tag1", "tag2"},
				"content":   "this is a test event",
				"sig":       "1234567890123456789012345678901234567890",
			},
			want: &gn.Event{
				ID:        "1234567890123456789012345678901234567890",
				PubKey:    "1234567890123456789012345678901234567890",
				CreatedAt: 1662612183,
				Kind:      1,
				Tags:      gn.Tags{"tag1", "tag2"},
				Content:   "this is a test event",
				Sig:       "1234567890123456789012345678901234567890",
			},
			wantErr: false,
		},
		// test case 2:
		{
			name: "test case 2",
			m: map[string]interface{}{
				"id":        "1234567890123456789012345678901234567890",
				"pubkey":    "1234567890123456789012345678901234567890",
				"created_at": 1662612183,
				
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

func Test_getTags(t *testing.T) {
	tests := []struct {
		name    string
		value   []interface{}
		want    gn.Tags
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTags(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("getTags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getTags() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getTS(t *testing.T) {
	tests := []struct {
		name    string
		value   interface{}
		want    gn.Timestamp
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTS(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("getTS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getTS() got = %v, want %v", got, tt.want)
			}
		})
	}
}
