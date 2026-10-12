package account

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dongwlin/dan-tick/internal/credential"
	"github.com/stretchr/testify/require"
)

const (
	sessionSecret = "fake_session_sensitive_value"
	csrfSecret    = "fake_csrf_sensitive_value"
)

type fakeCredentials struct {
	values credential.Credentials
	err    error
	calls  int
}

func (f *fakeCredentials) Load() (credential.Credentials, error) {
	f.calls++
	return f.values, f.err
}

func configured() *fakeCredentials {
	return &fakeCredentials{values: credential.Credentials{SESSDATA: sessionSecret, BiliJCT: csrfSecret}}
}

func testService(store *fakeCredentials, server *httptest.Server) *Service {
	service := NewService(store, server.Client())
	service.endpoint = server.URL + "/x/web-interface/nav"
	return service
}

func assertSafeResult(t *testing.T, status Status, err error) {
	t.Helper()
	serialized, marshalErr := json.Marshal(status)
	require.NoError(t, marshalErr)
	for _, secret := range []string{sessionSecret, csrfSecret} {
		require.NotContains(t, string(serialized), secret)
		if err != nil {
			require.NotContains(t, err.Error(), secret)
		}
	}
}

func TestCheckWithoutCredentialsDoesNotCallNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	store := &fakeCredentials{err: credential.ErrNotConfigured}
	result, err := testService(store, server).Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{State: Unconfigured}, result)
	require.Equal(t, 1, store.calls)
	require.Zero(t, requests.Load())
	assertSafeResult(t, result, err)
}

func TestCheckKeyringFailuresAreSafeAndDoNotCallNetwork(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()

	for name, test := range map[string]struct {
		storedError error
		want        error
	}{
		"unavailable": {credential.ErrUnavailable, credential.ErrUnavailable},
		"corrupt":     {credential.ErrCorrupt, credential.ErrCorrupt},
		"raw error":   {fmt.Errorf("system error: %s", sessionSecret), credential.ErrUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeCredentials{err: test.storedError}
			result, err := testService(store, server).Check(context.Background())
			require.Equal(t, Status{State: VerificationFailed}, result)
			require.ErrorIs(t, err, test.want)
			assertSafeResult(t, result, err)
		})
	}
	require.Zero(t, requests.Load())
}

func TestCheckSuccessfulLoginOnlySendsSessionCookie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/x/web-interface/nav", r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.Len(t, r.Cookies(), 1)
		cookie, err := r.Cookie("SESSDATA")
		require.NoError(t, err)
		require.Equal(t, sessionSecret, cookie.Value)
		require.NotContains(t, r.Header.Get("Cookie"), csrfSecret)
		_, _ = w.Write([]byte(`{"code":0,"data":{"isLogin":true,"mid":123456789,"uname":" Example ","face":"https://i0.hdslb.com/bfs/face/abc.jpg"}}`))
	}))
	defer server.Close()

	store := configured()
	status, err := testService(store, server).Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{State: LoggedIn, UID: "123456789", Name: "Example", AvatarURL: "https://i0.hdslb.com/bfs/face/abc.jpg"}, status)
	require.Equal(t, 1, store.calls)
	assertSafeResult(t, status, err)
}

func TestCheckProfileAllowsMissingFieldsAndRejectsUnsafeAvatars(t *testing.T) {
	for name, test := range map[string]struct {
		body       string
		wantAvatar string
	}{
		"missing profile":    {`{"code":0,"data":{"isLogin":true}}`, ""},
		"unsafe origin":      {`{"code":0,"data":{"isLogin":true,"mid":42,"face":"https://attacker.example/bfs/face/abc.jpg"}}`, ""},
		"upgrade CDN scheme": {`{"code":0,"data":{"isLogin":true,"mid":42,"face":"http://i0.hdslb.com/bfs/face/abc.jpg"}}`, "https://i0.hdslb.com/bfs/face/abc.jpg"},
		"unsafe path":        {`{"code":0,"data":{"isLogin":true,"mid":42,"face":"https://i0.hdslb.com/collect"}}`, ""},
		"strip query":        {`{"code":0,"data":{"isLogin":true,"mid":42,"face":"https://i0.hdslb.com/bfs/face/abc.jpg?token=private#secret"}}`, "https://i0.hdslb.com/bfs/face/abc.jpg"},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			status, err := testService(configured(), server).Check(context.Background())
			require.NoError(t, err)
			require.Equal(t, LoggedIn, status.State)
			require.Equal(t, test.wantAvatar, status.AvatarURL)
			assertSafeResult(t, status, err)
		})
	}
}

func TestCheckDoesNotEchoCredentialsInProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"isLogin":true,"mid":42,"uname":"` + sessionSecret + `","face":"https://i0.hdslb.com/bfs/face/` + csrfSecret + `.jpg"}}`))
	}))
	defer server.Close()
	status, err := testService(configured(), server).Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{State: LoggedIn, UID: "42"}, status)
	assertSafeResult(t, status, err)
}

func TestCheckExpiredLogin(t *testing.T) {
	for name, body := range map[string]string{
		"not logged in code": `{"code":-101,"message":"not logged in"}`,
		"not logged in flag": `{"code":0,"data":{"isLogin":false}}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			status, err := testService(configured(), server).Check(context.Background())
			require.NoError(t, err)
			require.Equal(t, Status{State: Expired}, status)
			assertSafeResult(t, status, err)
		})
	}
}

func TestCheckHTTPAndBusinessErrors(t *testing.T) {
	for name, test := range map[string]struct {
		code int
		body string
		want error
	}{
		"unauthorized HTTP": {http.StatusUnauthorized, `{"code":-101}`, ErrUnavailable},
		"rate limited":      {http.StatusTooManyRequests, `{"code":-101}`, ErrUnavailable},
		"server error":      {http.StatusBadGateway, `{"message":"` + sessionSecret + `"}`, ErrUnavailable},
		"unexpected code":   {http.StatusOK, `{"code":-400,"message":"` + csrfSecret + `"}`, ErrUnavailable},
		"no code":           {http.StatusOK, `{"data":{"isLogin":true}}`, ErrInvalidResponse},
		"missing data":      {http.StatusOK, `{"code":0}`, ErrInvalidResponse},
		"null login":        {http.StatusOK, `{"code":0,"data":{"isLogin":null}}`, ErrInvalidResponse},
		"wrong login type":  {http.StatusOK, `{"code":0,"data":{"isLogin":"true"}}`, ErrInvalidResponse},
		"malformed JSON":    {http.StatusOK, `{"code":0`, ErrInvalidResponse},
		"oversized body":    {http.StatusOK, strings.Repeat("x", maxResponseSize+1), ErrInvalidResponse},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.code)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			status, err := testService(configured(), server).Check(context.Background())
			require.Equal(t, Status{State: VerificationFailed}, status)
			require.ErrorIs(t, err, test.want)
			assertSafeResult(t, status, err)
		})
	}
}

func TestCheckNeverForwardsCookieOnRedirect(t *testing.T) {
	var forwarded atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
	}))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/collect", http.StatusFound)
	}))
	defer server.Close()

	status, err := testService(configured(), server).Check(context.Background())
	require.Equal(t, Status{State: VerificationFailed}, status)
	require.ErrorIs(t, err, ErrUnavailable)
	require.Zero(t, forwarded.Load())
	assertSafeResult(t, status, err)
}

func TestCheckDoesNotSendUnrelatedClientJarCookies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Len(t, r.Cookies(), 1)
		require.Equal(t, "SESSDATA", r.Cookies()[0].Name)
		_, _ = w.Write([]byte(`{"code":0,"data":{"isLogin":true}}`))
	}))
	defer server.Close()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{{Name: "unrelated", Value: csrfSecret}})
	service := NewService(configured(), &http.Client{Jar: jar})
	service.endpoint = server.URL
	status, err := service.Check(context.Background())
	require.NoError(t, err)
	require.Equal(t, Status{State: LoggedIn}, status)
	assertSafeResult(t, status, err)
}

func TestCheckCancelledAndTimedOutRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	service := testService(configured(), server)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := service.Check(cancelled)
	require.Equal(t, Status{State: VerificationFailed}, status)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, ErrUnavailable)
	assertSafeResult(t, status, err)

	timed, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	status, err = service.Check(timed)
	require.Equal(t, Status{State: VerificationFailed}, status)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, ErrUnavailable)
	assertSafeResult(t, status, err)
}

type leakingTransport struct{}

func (leakingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected transport error: " + sessionSecret)
}

func TestCheckDoesNotExposeTransportDiagnostics(t *testing.T) {
	service := NewService(configured(), &http.Client{Transport: leakingTransport{}})
	result, err := service.Check(context.Background())
	require.Equal(t, Status{State: VerificationFailed}, result)
	require.ErrorIs(t, err, ErrUnavailable)
	assertSafeResult(t, result, err)
}
