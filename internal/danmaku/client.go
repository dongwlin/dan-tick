// Package danmaku sends Bilibili live-room messages over HTTP. It does not
// schedule sends or implement account-wide rate limiting.
package danmaku

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/dongwlin/dan-tick/internal/credential"
)

const (
	sendURL         = "https://api.live.bilibili.com/msg/send"
	requestTimeout  = 10 * time.Second
	maxResponseSize = 1 << 20
)

var (
	ErrInvalidInput    = errors.New("invalid danmaku input")
	ErrAuthentication  = errors.New("Bilibili authentication failed")
	ErrRateLimited     = errors.New("Bilibili sending rate limit reached")
	ErrBadRequest      = errors.New("Bilibili rejected sending parameters")
	ErrTooLong         = errors.New("danmaku exceeds Bilibili length limit")
	ErrRejected        = errors.New("Bilibili rejected danmaku")
	ErrUncertain       = errors.New("danmaku send outcome uncertain")
	ErrTransport       = errors.New("danmaku transport failure")
	ErrTimeout         = errors.New("danmaku request timed out")
	ErrHTTPStatus      = errors.New("unexpected danmaku HTTP status")
	ErrInvalidResponse = errors.New("invalid danmaku response")
	ErrLoginCheck      = errors.New("login verification temporarily unavailable")
)

// Outcome describes what is actually known about the send side effect.
// Unknown MUST NOT be retried automatically: the server may have accepted it.
type Outcome string

const (
	NotSent  Outcome = "not_sent"
	Accepted Outcome = "accepted"
	Rejected Outcome = "rejected"
	Unknown  Outcome = "unknown"
)

// Result is safe for later execution history or UI use. Code is a business
// response code only, never an HTTP status or an untrusted upstream message.
type Result struct {
	Outcome Outcome `json:"outcome"`
	Code    *int    `json:"code,omitempty"`
}

// credentialLoader allows tests to replace the OS Keyring boundary.
type credentialLoader interface {
	Load() (credential.Credentials, error)
}

// A QR session holds its rotation gate for the complete authenticated request.
type credentialAcquirer interface {
	AcquireRequest(context.Context) (context.Context, credential.Credentials, func(), error)
}

type Client struct {
	store      credentialLoader
	httpClient *http.Client
	endpoint   string
	now        func() time.Time
}

func NewClient(store credentialLoader, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	// This client sends credentials. Do not include ambient jar cookies or
	// follow redirects (including same-origin redirects) with the POST body.
	safeClient := *httpClient
	safeClient.Jar = nil
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{store: store, httpClient: &safeClient, endpoint: sendURL, now: time.Now}
}

// Send performs exactly one POST. It does not retry, rate-limit, persist
// attempts, or return raw HTTP failures. C4 will wrap it with a shared limiter.
func (c *Client) Send(ctx context.Context, roomID, message string) (Result, error) {
	notSent := Result{Outcome: NotSent}
	if !validRoomID(roomID) || !validMessage(message) {
		return notSent, ErrInvalidInput
	}
	if err := ctx.Err(); err != nil {
		return notSent, err
	}
	var credentials credential.Credentials
	var err error
	owner, managed := c.store.(credentialAcquirer)
	if managed {
		var release func()
		var requestCtx context.Context
		requestCtx, credentials, release, err = owner.AcquireRequest(ctx)
		if err == nil {
			ctx = requestCtx
			defer release()
		}
	} else {
		credentials, err = c.store.Load()
	}
	if err != nil {
		switch {
		case errors.Is(err, credential.ErrReauthenticationRequired):
			return notSent, ErrAuthentication
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return notSent, err
		case errors.Is(err, credential.ErrNotConfigured):
			return notSent, credential.ErrNotConfigured
		case errors.Is(err, credential.ErrCorrupt):
			return notSent, credential.ErrCorrupt
		case errors.Is(err, credential.ErrUnavailable):
			return notSent, credential.ErrUnavailable
		default:
			if managed {
				return notSent, ErrLoginCheck
			}
			// A platform error may contain sensitive provider diagnostics.
			return notSent, credential.ErrUnavailable
		}
	}
	session := &http.Cookie{Name: "SESSDATA", Value: credentials.SESSDATA}
	csrfCookie := &http.Cookie{Name: "bili_jct", Value: credentials.BiliJCT}
	if session.Valid() != nil || csrfCookie.Valid() != nil ||
		credentials.SESSDATA == "" || credentials.BiliJCT == "" {
		return notSent, credential.ErrCorrupt
	}
	if err := ctx.Err(); err != nil {
		return notSent, err
	}

	values := url.Values{
		"roomid":   {roomID},
		"msg":      {message},
		"rnd":      {strconv.FormatInt(c.now().Unix(), 10)},
		"fontsize": {"25"},
		"color":    {"16777215"},
		"csrf":     {credentials.BiliJCT},
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return notSent, ErrInvalidInput
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(session)
	req.AddCookie(csrfCookie)
	if err := ctx.Err(); err != nil {
		return notSent, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		var netErr net.Error
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			return Result{Outcome: Unknown}, errors.Join(ErrUncertain, context.Canceled)
		case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded),
			errors.As(err, &netErr) && netErr.Timeout():
			return Result{Outcome: Unknown}, errors.Join(ErrUncertain, ErrTimeout)
		default:
			// Even a connection error cannot prove a POST was not processed.
			return Result{Outcome: Unknown}, errors.Join(ErrUncertain, ErrTransport)
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Result{Outcome: Unknown}, errors.Join(ErrUncertain, ErrHTTPStatus)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		var netErr net.Error
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			return Result{Outcome: Unknown}, errors.Join(ErrUncertain, context.Canceled)
		case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded),
			errors.As(err, &netErr) && netErr.Timeout():
			return Result{Outcome: Unknown}, errors.Join(ErrUncertain, ErrTimeout)
		default:
			return Result{Outcome: Unknown}, errors.Join(ErrUncertain, ErrInvalidResponse)
		}
	}
	if len(body) > maxResponseSize {
		return Result{Outcome: Unknown}, errors.Join(ErrUncertain, ErrInvalidResponse)
	}
	var data struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal(body, &data) != nil || data.Code == nil {
		return Result{Outcome: Unknown}, errors.Join(ErrUncertain, ErrInvalidResponse)
	}

	result := Result{Outcome: Rejected, Code: data.Code}
	switch *data.Code {
	case 0:
		result.Outcome = Accepted
		return result, nil
	case -101, -111:
		return result, ErrAuthentication
	case 10031:
		return result, ErrRateLimited
	case -400:
		return result, ErrBadRequest
	case 1003212:
		return result, ErrTooLong
	default:
		return result, ErrRejected
	}
}

func validRoomID(id string) bool {
	parsed, err := strconv.ParseInt(id, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == id
}

func validMessage(message string) bool {
	if strings.TrimSpace(message) == "" {
		return false
	}
	for _, r := range message {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
