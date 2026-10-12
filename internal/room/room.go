// Package room resolves Bilibili live room inputs to canonical platform IDs.
// It does not persist rooms or depend on Wails.
package room

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	roomInfoURL     = "https://api.live.bilibili.com/room/v1/Room/get_info"
	masterInfoURL   = "https://api.live.bilibili.com/live_user/v1/Master/info"
	requestTimeout  = 10 * time.Second
	maxResponseSize = 1 << 20
)

var (
	ErrInvalidInput    = errors.New("invalid live room ID or URL")
	ErrNotFound        = errors.New("live room not found")
	ErrUnavailable     = errors.New("live room service unavailable")
	ErrInvalidResponse = errors.New("invalid live room response")
)

// Info contains the canonical ID for persistence and optional display metadata.
// Title and AnchorName may be empty when the upstream does not provide them.
type Info struct {
	RoomID     string
	Title      string
	AnchorName string
}

type roomData struct {
	RoomID    int64  `json:"room_id"`
	UID       int64  `json:"uid"`
	Title     string `json:"title"`
	UserCover string `json:"user_cover"`
	Cover     string `json:"cover"`
}

// ParseInput accepts a numeric short/long room ID or a direct Bilibili live
// room URL. It does not follow untrusted links or resolve third-party short URLs.
func ParseInput(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ErrInvalidInput
	}
	if isDigits(input) {
		return canonicalID(input)
	}

	u, err := url.Parse(input)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" {
		return "", ErrInvalidInput
	}
	switch strings.ToLower(u.Hostname()) {
	case "live.bilibili.com", "m.live.bilibili.com":
	default:
		return "", ErrInvalidInput
	}
	if u.RawPath != "" || !strings.HasPrefix(u.Path, "/") {
		return "", ErrInvalidInput
	}
	path := strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/")
	// Both the standard room link and the embedded /blanc/ link are used by
	// Bilibili's live site. Reject unrelated site paths rather than guessing IDs.
	path = strings.TrimPrefix(path, "blanc/")
	if !isDigits(path) {
		return "", ErrInvalidInput
	}
	return canonicalID(path)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func canonicalID(raw string) (string, error) {
	if !isDigits(raw) {
		return "", ErrInvalidInput
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return "", ErrInvalidInput
	}
	return strconv.FormatInt(id, 10), nil
}

// Client only queries the public Bilibili endpoints; no credentials are used.
type Client struct {
	httpClient *http.Client
	roomURL    string
	masterURL  string
	timeout    time.Duration
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	return &Client{httpClient: httpClient, roomURL: roomInfoURL, masterURL: masterInfoURL, timeout: requestTimeout}
}

// Resolve validates an input, queries its real room ID, and optionally obtains
// a title and anchor name. Failed metadata queries do not invalidate a room.
// Callers should use RoomID, not the supplied short number, as the stable key.
func (c *Client) Resolve(ctx context.Context, input string) (Info, error) {
	id, err := ParseInput(input)
	if err != nil {
		return Info{}, err
	}
	data, err := c.getRoomData(ctx, id)
	if err != nil {
		return Info{}, err
	}
	info := Info{
		RoomID: strconv.FormatInt(data.RoomID, 10),
		Title:  data.Title,
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	if data.UID == 0 {
		return info, nil
	}
	var master struct {
		Code *int `json:"code"`
		Data *struct {
			Info *struct {
				Uname string `json:"uname"`
			} `json:"info"`
		} `json:"data"`
	}
	if err := c.getJSON(ctx, c.masterURL, "uid", strconv.FormatInt(data.UID, 10), &master); err == nil &&
		master.Code != nil && *master.Code == 0 && master.Data != nil && master.Data.Info != nil {
		info.AnchorName = master.Data.Info.Uname
	}
	if err := ctx.Err(); err != nil {
		return Info{}, err
	}
	return info, nil
}

// Cover looks up only optional artwork, without requesting the anchor name.
// Empty or untrusted image URLs are treated as missing artwork.
func (c *Client) Cover(ctx context.Context, roomID string) (string, error) {
	id, err := canonicalID(roomID)
	if err != nil {
		return "", err
	}
	data, err := c.getRoomData(ctx, id)
	if err != nil {
		return "", err
	}
	for _, candidate := range []string{data.UserCover, data.Cover} {
		if cover := safeCoverURL(candidate); cover != "" {
			return cover, nil
		}
	}
	return "", nil
}

// Cover URLs are controlled by Bilibili, not users. Only display HTTPS images
// from its image CDN, never arbitrary URLs supplied by an upstream response.
func safeCoverURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Port() != "" ||
		!strings.HasSuffix(strings.ToLower(u.Hostname()), ".hdslb.com") {
		return ""
	}
	u.Scheme = "https"
	u.Fragment = ""
	return u.String()
}

func (c *Client) getRoomData(ctx context.Context, id string) (roomData, error) {
	var response struct {
		Code *int      `json:"code"`
		Data *roomData `json:"data"`
	}
	if err := c.getJSON(ctx, c.roomURL, "room_id", id, &response); err != nil {
		return roomData{}, fmt.Errorf("query live room: %w", err)
	}
	if response.Code == nil {
		return roomData{}, fmt.Errorf("query live room: %w: missing code", ErrInvalidResponse)
	}
	if *response.Code == 1 {
		return roomData{}, ErrNotFound
	}
	if *response.Code != 0 {
		return roomData{}, fmt.Errorf("%w: room info code %d", ErrUnavailable, *response.Code)
	}
	if response.Data == nil || response.Data.RoomID <= 0 || response.Data.UID < 0 {
		return roomData{}, fmt.Errorf("query live room: %w: missing or invalid ID", ErrInvalidResponse)
	}
	return *response.Data, nil
}

func (c *Client) getJSON(ctx context.Context, endpoint, key, value string, dest any) error {
	// Each endpoint has its own deadline. An optional metadata timeout must
	// not cancel the caller or invalidate an already verified room.
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("%w: endpoint", ErrInvalidResponse)
	}
	params := u.Query()
	params.Set(key, value)
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%w: request", ErrInvalidResponse)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return fmt.Errorf("%w: read body: %w", ErrInvalidResponse, err)
	}
	if len(body) > maxResponseSize || json.Unmarshal(body, dest) != nil {
		return fmt.Errorf("%w: malformed or oversized JSON", ErrInvalidResponse)
	}
	return nil
}
