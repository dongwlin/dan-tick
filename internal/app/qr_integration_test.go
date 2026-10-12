package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/account"
	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/dongwlin/dan-tick/internal/danmaku"
	"github.com/dongwlin/dan-tick/internal/desktop"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

// The fake vault only retains one encrypted-store item boundary; it never
// writes test credentials into SQLite or a plaintext fixture on disk.
type qrRuntimeKeyring struct {
	mu        sync.Mutex
	secret    string
	present   bool
	writes    int
	failWrite int
}

func (k *qrRuntimeKeyring) Set(_, _, secret string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.writes++
	if k.writes == k.failWrite {
		return errors.New("vault inaccessible: private_session_and_token")
	}
	k.secret, k.present = secret, true
	return nil
}

func (k *qrRuntimeKeyring) Get(_, _ string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if !k.present {
		return "", keyring.ErrNotFound
	}
	return k.secret, nil
}

func (k *qrRuntimeKeyring) Delete(_, _ string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.secret, k.present = "", false
	return nil
}

func qrRuntimeCredentials(prefix string) credential.Credentials {
	return credential.Credentials{
		Version: 1, Source: "qr", UID: "123", RefreshState: "ready",
		SESSDATA: prefix + "_session", BiliJCT: prefix + "_csrf", RefreshToken: prefix + "_token",
	}
}

func qrRuntimeResponse(req *http.Request, body string, c *credential.Credentials) *http.Response {
	response := &http.Response{
		StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(body)), Request: req,
	}
	if c != nil {
		for _, cookie := range []*http.Cookie{
			{Name: "SESSDATA", Value: c.SESSDATA}, {Name: "bili_jct", Value: c.BiliJCT},
			{Name: "DedeUserID", Value: c.UID},
		} {
			response.Header.Add("Set-Cookie", cookie.String())
		}
	}
	return response
}

type qrRuntimeProtocol struct {
	mu           sync.Mutex
	paths        map[string]int
	needRefresh  bool
	blockPath    string
	started      chan struct{}
	lastSend     string
	lastSendCSRF string
}

func (p *qrRuntimeProtocol) client() *http.Client {
	return &http.Client{Transport: transport(p.roundTrip)}
}

func (p *qrRuntimeProtocol) count(path string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.paths[path]
}

func (p *qrRuntimeProtocol) roundTrip(req *http.Request) (*http.Response, error) {
	path := req.URL.Path
	p.mu.Lock()
	if p.paths == nil {
		p.paths = make(map[string]int)
	}
	p.paths[path]++
	p.mu.Unlock()
	if path == p.blockPath {
		select {
		case p.started <- struct{}{}:
		default:
		}
		<-req.Context().Done()
		return nil, req.Context().Err()
	}
	qr := qrRuntimeCredentials("qr")
	next := qrRuntimeCredentials("rotated")
	switch {
	case path == "/x/passport-login/web/qrcode/generate":
		return qrRuntimeResponse(req, `{"code":0,"data":{"url":"https://passport.bilibili.com/qrcode/h5/login?qrcode_key=isolated_key","qrcode_key":"isolated_key"}}`, nil), nil
	case path == "/x/passport-login/web/qrcode/poll":
		return qrRuntimeResponse(req, `{"code":0,"data":{"code":0,"refresh_token":"qr_token"}}`, &qr), nil
	case path == "/x/web-interface/nav":
		return qrRuntimeResponse(req, `{"code":0,"data":{"isLogin":true,"mid":123,"uname":"Test account"}}`, nil), nil
	case path == "/x/passport-login/web/cookie/info":
		if p.needRefresh {
			return qrRuntimeResponse(req, `{"code":0,"data":{"refresh":true,"timestamp":1700000000123}}`, nil), nil
		}
		return qrRuntimeResponse(req, `{"code":0,"data":{"refresh":false,"timestamp":1700000000123}}`, nil), nil
	case strings.HasPrefix(path, "/correspond/1/"):
		return qrRuntimeResponse(req, `<html><div id="1-name">refresh_csrf</div></html>`, nil), nil
	case path == "/x/passport-login/web/cookie/refresh":
		return qrRuntimeResponse(req, `{"code":0,"data":{"status":0,"refresh_token":"rotated_token"}}`, &next), nil
	case path == "/x/passport-login/web/confirm/refresh":
		return qrRuntimeResponse(req, `{"code":0}`, nil), nil
	case path == "/msg/send":
		if err := req.ParseForm(); err != nil {
			return nil, err
		}
		p.mu.Lock()
		p.lastSend, p.lastSendCSRF = req.Header.Get("Cookie"), req.PostForm.Get("csrf")
		p.mu.Unlock()
		return qrRuntimeResponse(req, `{"code":0}`, nil), nil
	default:
		return nil, errors.New("unexpected simulated authentication request")
	}
}

func TestQRRuntimeUsesPersistedSessionForAccountAndSharedSender(t *testing.T) {
	ctx := context.Background()
	db := runtimeDB(t)
	saved := saveTask(t, db, time.Now().UTC().Add(time.Hour).Truncate(time.Second))
	vault := &qrRuntimeKeyring{}
	store := credential.NewStoreWithBackend(vault)
	legacy := credential.Credentials{SESSDATA: "legacy_session", BiliJCT: "legacy_csrf"}
	require.NoError(t, store.Save(legacy))
	protocol := &qrRuntimeProtocol{}
	runtime := newRuntimeWithHTTP(db, nil, store, protocol.client())
	defer runtime.Close()
	var changes atomic.Int32
	runtime.BindEvents(func(event string) {
		if event == desktop.EventAccountChanged {
			changes.Add(1)
		}
	})

	// Existing manual credentials survive migration and never reach msg/send.
	result, err := runtime.Operations.TestSend(ctx, desktop.TestSendInput{RoomID: saved.RoomID, Message: "test message"})
	require.NoError(t, err)
	require.Equal(t, desktop.TestSendResult{Outcome: danmaku.NotSent, ErrorCode: "authentication"}, result)
	require.Zero(t, protocol.count("/msg/send"))
	require.Equal(t, account.ReauthenticationRequired, runtime.Operations.CheckAccount(ctx).State)

	challenge, err := runtime.Operations.BeginQRLogin(ctx)
	require.NoError(t, err)
	require.Equal(t, "waiting", challenge.State)
	login, err := runtime.Operations.PollQRLogin(ctx)
	require.NoError(t, err)
	require.Equal(t, "confirmed", login.State)
	persisted, err := credential.NewStoreWithBackend(vault).Load()
	require.NoError(t, err)
	require.Equal(t, qrRuntimeCredentials("qr"), persisted)
	require.Equal(t, 2, protocol.count("/x/web-interface/nav"), "both the candidate and legacy UID are verified")
	require.EqualValues(t, 1, changes.Load())

	status := runtime.Operations.CheckAccount(ctx)
	require.Equal(t, account.Status{State: account.LoggedIn, UID: "123", Name: "Test account"}, status)
	result, err = runtime.Operations.TestSend(ctx, desktop.TestSendInput{RoomID: saved.RoomID, Message: "test message"})
	require.NoError(t, err)
	require.Equal(t, desktop.TestSendResult{Outcome: danmaku.Accepted}, result)
	require.Equal(t, 1, protocol.count("/msg/send"))
	protocol.mu.Lock()
	cookie, csrf := protocol.lastSend, protocol.lastSendCSRF
	protocol.mu.Unlock()
	require.Contains(t, cookie, "SESSDATA=qr_session")
	require.Contains(t, cookie, "bili_jct=qr_csrf")
	require.Equal(t, "qr_csrf", csrf)
	require.NotContains(t, cookie, "legacy")

	var executionCount int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM executions").Scan(&executionCount))
	require.Zero(t, executionCount, "the shared test sender does not create task executions")
}

func TestQRRuntimeKeyringFailuresNeverPublishCandidateOrSend(t *testing.T) {
	for _, boundary := range []string{"qr", "refresh"} {
		t.Run(boundary, func(t *testing.T) {
			ctx := context.Background()
			db := runtimeDB(t)
			vault := &qrRuntimeKeyring{}
			store := credential.NewStoreWithBackend(vault)
			initial := qrRuntimeCredentials("old")
			protocol := &qrRuntimeProtocol{needRefresh: boundary == "refresh"}
			if boundary == "qr" {
				initial = credential.Credentials{SESSDATA: "legacy_session", BiliJCT: "legacy_csrf"}
				vault.failWrite = 2
			} else {
				vault.failWrite = 3 // The durable old/pending write succeeds; new/pending fails.
			}
			require.NoError(t, store.Save(initial))
			runtime := newRuntimeWithHTTP(db, nil, store, protocol.client())
			defer runtime.Close()
			if boundary == "qr" {
				_, err := runtime.Operations.BeginQRLogin(ctx)
				require.NoError(t, err)
				_, err = runtime.Operations.PollQRLogin(ctx)
				require.Error(t, err)
				require.Contains(t, string(desktop.MarshalError(err)), "keyring_unavailable")
				require.NotContains(t, err.Error(), "private_session_and_token")
				login, err := runtime.Operations.PollQRLogin(ctx)
				require.NoError(t, err)
				require.Equal(t, "cancelled", login.State)
			} else {
				status := runtime.Operations.CheckAccount(ctx)
				require.Equal(t, account.ReauthenticationRequired, status.State)
				initial.RefreshState = "pending"
				require.Equal(t, 1, protocol.count("/x/passport-login/web/cookie/refresh"))
				require.Zero(t, protocol.count("/x/passport-login/web/confirm/refresh"))
			}
			persisted, err := credential.NewStoreWithBackend(vault).Load()
			require.NoError(t, err)
			require.Equal(t, initial, persisted)
			result, err := runtime.Sender.TestSend(ctx, "12345", "test message")
			require.ErrorIs(t, err, danmaku.ErrAuthentication)
			require.Equal(t, danmaku.NotSent, result.Outcome)
			require.Zero(t, protocol.count("/msg/send"))
			public, err := json.Marshal(runtime.Operations.CheckAccount(ctx))
			require.NoError(t, err)
			require.NotContains(t, string(public), initial.SESSDATA)
			require.NotContains(t, string(public), initial.BiliJCT)
			require.NotContains(t, string(public), "old_token")
		})
	}
}

func TestQRRuntimeCloseCancelsAuthenticationHTTP(t *testing.T) {
	for _, boundary := range []string{"background check", "send preflight", "send post"} {
		t.Run(boundary, func(t *testing.T) {
			db := runtimeDB(t)
			vault := &qrRuntimeKeyring{}
			store := credential.NewStoreWithBackend(vault)
			require.NoError(t, store.Save(qrRuntimeCredentials("old")))
			protocol := &qrRuntimeProtocol{
				blockPath: "/x/passport-login/web/cookie/info", started: make(chan struct{}, 1),
			}
			if boundary == "send post" {
				protocol.blockPath = "/msg/send"
			}
			runtime := newRuntimeWithHTTP(db, nil, store, protocol.client())
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer func() { cancel(); runtime.Close() }()
			finished := make(chan desktop.TestSendResult, 1)
			if boundary == "background check" {
				require.NoError(t, runtime.Start(ctx))
			} else {
				saved := saveTask(t, db, time.Now().UTC().Add(time.Hour).Truncate(time.Second))
				go func() {
					result, _ := runtime.Operations.TestSend(ctx, desktop.TestSendInput{RoomID: saved.RoomID, Message: "test message"})
					finished <- result
				}()
			}
			select {
			case <-protocol.started:
			case <-time.After(time.Second):
				t.Fatal("the assembled session did not reach its authentication HTTP boundary")
			}
			closed := make(chan struct{})
			go func() { runtime.Close(); close(closed) }()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("runtime shutdown did not cancel the pending authentication HTTP")
			}
			require.Equal(t, 1, protocol.count("/x/passport-login/web/cookie/info"))
			require.Zero(t, protocol.count("/x/passport-login/web/cookie/refresh"))
			if boundary == "background check" {
				require.Zero(t, protocol.count("/msg/send"))
			} else {
				result := <-finished
				require.Equal(t, "cancelled", result.ErrorCode)
				if boundary == "send post" {
					require.Equal(t, danmaku.Unknown, result.Outcome)
					require.Equal(t, 1, protocol.count("/msg/send"))
				} else {
					require.Equal(t, danmaku.NotSent, result.Outcome)
					require.Zero(t, protocol.count("/msg/send"))
				}
			}
			persisted, err := store.Load()
			require.NoError(t, err)
			require.Equal(t, qrRuntimeCredentials("old"), persisted)
		})
	}
}
