package room

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseInput(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1", "1"},
		{" 0000123 ", "123"},
		{"https://live.bilibili.com/123", "123"},
		{"https://live.bilibili.com/0001/?visit_id=demo#room", "1"},
		{"http://m.live.bilibili.com/987", "987"},
		{"https://live.bilibili.com/blanc/44", "44"},
		{"9223372036854775807", "9223372036854775807"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseInput(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseInputRejectsInvalidURLsAndIDs(t *testing.T) {
	for _, input := range []string{
		"", " ", "0", "000", "-1", "+1", "1.5", "一", "9223372036854775808",
		"https://live.bilibili.com/0", "https://live.bilibili.com/abc",
		"https://live.bilibili.com/1/other", "https://live.bilibili.com/1//",
		"https://live.bilibili.com/%31", "https://live.bilibili.com/1:80",
		"https://live.bilibili.com.evil.example/1", "https://bilibili.com/1",
		"https://live.bilibili.com@evil.example/1", "https://user@live.bilibili.com/1",
		"https://live.bilibili.com:443/1", "ftp://live.bilibili.com/1",
		"//live.bilibili.com/1", "https://b23.tv/abc", "https://localhost/1",
	} {
		t.Run(input, func(t *testing.T) {
			_, err := ParseInput(input)
			require.ErrorIs(t, err, ErrInvalidInput)
		})
	}
}

func TestResolveNormalizesShortIDAndGetsDisplayInfo(t *testing.T) {
	roomRequests, masterRequests := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Cookie"))
		switch r.URL.Path {
		case "/room/v1/Room/get_info":
			roomRequests++
			assert.Equal(t, "1", r.URL.Query().Get("room_id"))
			fmt.Fprint(w, `{"code":0,"data":{"room_id":5440,"short_id":1,"uid":9617619,"title":"测试直播间"}}`)
		case "/live_user/v1/Master/info":
			masterRequests++
			assert.Equal(t, "9617619", r.URL.Query().Get("uid"))
			fmt.Fprint(w, `{"code":0,"data":{"info":{"uname":"测试主播"}}}`)
		default:
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := testClient(server)
	info, err := client.Resolve(context.Background(), "https://live.bilibili.com/0001?visit_id=abc")
	require.NoError(t, err)
	assert.Equal(t, Info{RoomID: "5440", Title: "测试直播间", AnchorName: "测试主播"}, info)
	assert.Equal(t, 1, roomRequests)
	assert.Equal(t, 1, masterRequests)
}

func TestResolveShortAndLongIDsShareCanonicalIdentity(t *testing.T) {
	var queried []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queried = append(queried, r.URL.Query().Get("room_id"))
		fmt.Fprint(w, `{"code":0,"data":{"room_id":5440}}`)
	}))
	defer server.Close()
	client := testClient(server)
	short, err := client.Resolve(context.Background(), "1")
	require.NoError(t, err)
	long, err := client.Resolve(context.Background(), "https://live.bilibili.com/5440")
	require.NoError(t, err)
	assert.Equal(t, short.RoomID, long.RoomID)
	assert.Equal(t, "5440", short.RoomID)
	assert.Equal(t, []string{"1", "5440"}, queried)
}

func TestResolveOptionalMetadata(t *testing.T) {
	for _, tt := range []struct {
		name, roomBody string
		masterStatus   int
		masterBody     string
		wantTitle      string
		masterCalled   bool
	}{
		{"no uid or title", `{"code":0,"data":{"room_id":123}}`, 0, "", "", false},
		{"master rejected", `{"code":0,"data":{"room_id":123,"uid":8,"title":"离线房间"}}`, 200, `{"code":1}`, "离线房间", true},
		{"master unavailable", `{"code":0,"data":{"room_id":123,"uid":8}}`, 503, "", "", true},
		{"master missing info", `{"code":0,"data":{"room_id":123,"uid":8}}`, 200, `{"code":0,"data":{}}`, "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			masterCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/room/v1/Room/get_info" {
					fmt.Fprint(w, tt.roomBody)
					return
				}
				masterCalled = true
				w.WriteHeader(tt.masterStatus)
				fmt.Fprint(w, tt.masterBody)
			}))
			defer server.Close()
			info, err := testClient(server).Resolve(context.Background(), "123")
			require.NoError(t, err)
			assert.Equal(t, "123", info.RoomID)
			assert.Equal(t, tt.wantTitle, info.Title)
			assert.Empty(t, info.AnchorName)
			assert.Equal(t, tt.masterCalled, masterCalled)
		})
	}
}

func TestResolveRoomFailures(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		status     int
		want       error
	}{
		{"room does not exist", `{"code":1}`, 200, ErrNotFound},
		{"upstream refuses", `{"code":-400}`, 200, ErrUnavailable},
		{"service failure", `server is down`, 502, ErrUnavailable},
		{"missing code", `{"data":{"room_id":99}}`, 200, ErrInvalidResponse},
		{"missing data", `{"code":0}`, 200, ErrInvalidResponse},
		{"missing real ID", `{"code":0,"data":{"uid":2}}`, 200, ErrInvalidResponse},
		{"negative ID", `{"code":0,"data":{"room_id":-1}}`, 200, ErrInvalidResponse},
		{"invalid JSON", `{"code":0`, 200, ErrInvalidResponse},
		{"invalid type", `{"code":0,"data":{"room_id":"123"}}`, 200, ErrInvalidResponse},
		{"oversized response", `{"code":0,"data":{"room_id":123},"padding":"` + strings.Repeat("a", maxResponseSize) + `"}`, 200, ErrInvalidResponse},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				fmt.Fprint(w, tt.body)
			}))
			defer server.Close()
			info, err := testClient(server).Resolve(context.Background(), "9")
			assert.Equal(t, Info{}, info)
			require.ErrorIs(t, err, tt.want)
		})
	}
}

func TestResolveRejectsInvalidInputWithoutRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("invalid input must not cause network traffic")
	}))
	defer server.Close()
	_, err := testClient(server).Resolve(context.Background(), "https://not-bilibili.example/1")
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestResolveRespectsCancellationAndTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := testClient(server)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := client.Resolve(ctx, "1")
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "timeout must be identifiable: %v", err)
}

func testClient(server *httptest.Server) *Client {
	client := NewClient(server.Client())
	client.roomURL = server.URL + "/room/v1/Room/get_info"
	client.masterURL = server.URL + "/live_user/v1/Master/info"
	return client
}
