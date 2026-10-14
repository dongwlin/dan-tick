package danmaku

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	sessionSecret = "fake_session_secret"
	csrfSecret    = "fake_csrf_secret"
)

type fakeStore struct {
	creds credential.Credentials
	err   error
	loads int
}

func (f *fakeStore) Load() (credential.Credentials, error) {
	f.loads++
	return f.creds, f.err
}

func storedCredentials() *fakeStore {
	return &fakeStore{creds: credential.Credentials{SESSDATA: sessionSecret, BiliJCT: csrfSecret}}
}

func testClient(store *fakeStore, server *httptest.Server) *Client {
	client := NewClient(store, server.Client())
	client.endpoint = server.URL + "/msg/send"
	client.now = func() time.Time { return time.Unix(1760123456, 0) }
	return client
}

func safe(t *testing.T, result Result, err error) {
	t.Helper()
	payload, marshalErr := json.Marshal(result)
	require.NoError(t, marshalErr)
	for _, secret := range []string{sessionSecret, csrfSecret} {
		require.NotContains(t, string(payload), secret)
		if err != nil {
			require.NotContains(t, err.Error(), secret)
		}
	}
}

func TestSendAcceptedUsesOnlyDocumentedParameters(t *testing.T) {
	message := strings.Repeat("a", 67) + "  弹 &+ 🐱 " // Exactly 80 weighted units.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/msg/send", r.URL.Path)
		require.Empty(t, r.URL.RawQuery)
		require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
		require.Equal(t, "application/json", r.Header.Get("Accept"))
		require.Len(t, r.Cookies(), 2)
		session, err := r.Cookie("SESSDATA")
		require.NoError(t, err)
		require.Equal(t, sessionSecret, session.Value)
		csrf, err := r.Cookie("bili_jct")
		require.NoError(t, err)
		require.Equal(t, csrfSecret, csrf.Value)

		require.NoError(t, r.ParseForm())
		require.Equal(t, url.Values{
			"roomid":   {"12345"},
			"msg":      {message},
			"rnd":      {"1760123456"},
			"fontsize": {"25"},
			"color":    {"16777215"},
			"csrf":     {csrfSecret},
		}, r.PostForm)
		_, _ = w.Write([]byte(`{"code":0,"message":"0","data":{"other":"ignored"}}`))
	}))
	defer server.Close()

	store := storedCredentials()
	result, err := testClient(store, server).Send(context.Background(), "12345", message)
	require.NoError(t, err)
	require.Equal(t, Accepted, result.Outcome)
	require.NotNil(t, result.Code)
	require.Zero(t, *result.Code)
	require.Equal(t, 1, store.loads)
	require.EqualValues(t, 1, calls.Load())
	safe(t, result, err)
}

func TestSendClassifiesBusinessResponses(t *testing.T) {
	for name, tc := range map[string]struct {
		code int
		want error
	}{
		"login expired":  {-101, ErrAuthentication},
		"csrf rejected":  {-111, ErrAuthentication},
		"rate limited":   {10031, ErrRateLimited},
		"bad request":    {-400, ErrBadRequest},
		"message length": {1003212, ErrTooLong},
		"other negative": {-500, ErrRejected},
		"other positive": {99999, ErrRejected},
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = fmt.Fprintf(w, `{"code":%d,"message":"%s","msg":"%s"}`, tc.code, sessionSecret, csrfSecret)
			}))
			defer server.Close()
			result, err := testClient(storedCredentials(), server).Send(context.Background(), "42", "test")
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, Rejected, result.Outcome)
			require.Equal(t, tc.code, *result.Code)
			require.EqualValues(t, 1, requests.Load())
			safe(t, result, err)
		})
	}
}

func TestSendNeverTreatsHTTPStatusOrMalformedResponseAsAccepted(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"HTTP unauthorized":  {http.StatusUnauthorized, `{"code":-101}`, ErrHTTPStatus},
		"HTTP rate limit":    {http.StatusTooManyRequests, `{"code":10031}`, ErrHTTPStatus},
		"HTTP server error":  {http.StatusBadGateway, sessionSecret, ErrHTTPStatus},
		"no code":            {http.StatusOK, `{"data":{}}`, ErrInvalidResponse},
		"null code":          {http.StatusOK, `{"code":null}`, ErrInvalidResponse},
		"wrong type":         {http.StatusOK, `{"code":"0"}`, ErrInvalidResponse},
		"invalid JSON":       {http.StatusOK, `{"code":0`, ErrInvalidResponse},
		"HTML error page":    {http.StatusOK, `<html>no</html>`, ErrInvalidResponse},
		"oversized response": {http.StatusOK, strings.Repeat("z", maxResponseSize+1), ErrInvalidResponse},
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			result, err := testClient(storedCredentials(), server).Send(context.Background(), "42", "test")
			require.Equal(t, Unknown, result.Outcome)
			require.Nil(t, result.Code)
			require.ErrorIs(t, err, ErrUncertain)
			require.ErrorIs(t, err, tc.want)
			require.EqualValues(t, 1, requests.Load())
			safe(t, result, err)
		})
	}
}

func TestSendRejectsInvalidInputBeforeLoadingKeyring(t *testing.T) {
	for name, tc := range map[string]struct {
		room, message string
	}{
		"no room":          {"", "test"},
		"negative room":    {"-4", "test"},
		"room with zeros":  {"0004", "test"},
		"room not numeric": {"12a", "test"},
		"room overflow":    {"9223372036854775808", "test"},
		"empty message":    {"42", ""},
		"space only":       {"42", " \t "},
		"newlines":         {"42", "two\nlines"},
		"control chars":    {"42", "text\x00"},
		"invalid UTF8":     {"42", string([]byte{0xff})},
	} {
		t.Run(name, func(t *testing.T) {
			store := storedCredentials()
			client := NewClient(store, nil)
			result, err := client.Send(context.Background(), tc.room, tc.message)
			require.ErrorIs(t, err, ErrInvalidInput)
			require.Equal(t, NotSent, result.Outcome)
			require.Zero(t, store.loads)
		})
	}
}

func TestSendRejectsOverlongMessagesBeforeCredentialsOrHTTP(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer server.Close()
	for _, tc := range []struct {
		text  string
		count int
	}{{"a", 81}, {"弹", 41}, {"🐱", 21}} {
		t.Run(tc.text, func(t *testing.T) {
			store := storedCredentials()
			client := testClient(store, server)
			result, err := client.Send(context.Background(), "42", strings.Repeat(tc.text, tc.count))
			require.ErrorIs(t, err, ErrTooLong)
			require.Equal(t, NotSent, result.Outcome)
			require.Zero(t, store.loads)
		})
	}
	require.Zero(t, requests.Load())
}

func TestSendKeyringErrorsDoNotIssueRequests(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	for name, tc := range map[string]struct {
		err  error
		want error
	}{
		"unconfigured": {credential.ErrNotConfigured, credential.ErrNotConfigured},
		"corrupt":      {credential.ErrCorrupt, credential.ErrCorrupt},
		"unavailable":  {credential.ErrUnavailable, credential.ErrUnavailable},
		"unexpected":   {errors.New(sessionSecret), credential.ErrUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{err: tc.err}
			result, err := testClient(store, server).Send(context.Background(), "42", "test")
			require.Equal(t, NotSent, result.Outcome)
			require.ErrorIs(t, err, tc.want)
			safe(t, result, err)
		})
	}
	store := &fakeStore{creds: credential.Credentials{SESSDATA: "a;bad", BiliJCT: csrfSecret}}
	result, err := testClient(store, server).Send(context.Background(), "42", "test")
	require.Equal(t, NotSent, result.Outcome)
	require.ErrorIs(t, err, credential.ErrCorrupt)
	require.Zero(t, requests.Load())
}

func TestSendDoesNotFollowRedirectOrSendAmbientCookies(t *testing.T) {
	var redirected atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
	}))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Len(t, r.Cookies(), 2)
		http.Redirect(w, r, other.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{{Name: "ambient", Value: "should_not_send"}})
	service := NewClient(storedCredentials(), &http.Client{Jar: jar})
	service.endpoint = server.URL
	result, err := service.Send(context.Background(), "42", "test")
	require.Equal(t, Unknown, result.Outcome)
	require.ErrorIs(t, err, ErrHTTPStatus)
	require.Zero(t, redirected.Load())
	safe(t, result, err)
}

func TestSendTimeoutAndCancellationHaveUncertainOutcome(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	client := testClient(storedCredentials(), server)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	result, err := client.Send(ctx, "42", "test")
	require.Equal(t, Unknown, result.Outcome)
	require.ErrorIs(t, err, ErrUncertain)
	require.ErrorIs(t, err, context.Canceled)
	safe(t, result, err)

	timed, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	result, err = client.Send(timed, "42", "test")
	require.Equal(t, Unknown, result.Outcome)
	require.ErrorIs(t, err, ErrUncertain)
	require.ErrorIs(t, err, ErrTimeout)
	safe(t, result, err)
}

func TestSendTimesOutWhileReadingResponseBody(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	ctx, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	result, err := testClient(storedCredentials(), server).Send(ctx, "42", "test")
	require.Equal(t, Unknown, result.Outcome)
	require.ErrorIs(t, err, ErrUncertain)
	require.ErrorIs(t, err, ErrTimeout)
	safe(t, result, err)
}

func TestSendAlreadyCancelledIsNotSent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := storedCredentials()
	result, err := NewClient(store, nil).Send(ctx, "42", "test")
	require.Equal(t, NotSent, result.Outcome)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, store.loads)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New(sessionSecret) }

func TestSendHandlesBrokenResponseBodyAndTransportErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		rt   roundTripFunc
		want error
	}{
		"transport error": {func(*http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("connection error: %s", csrfSecret)
		}, ErrTransport},
		"body error": {func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(brokenReader{}), Header: make(http.Header)}, nil
		}, ErrInvalidResponse},
	} {
		t.Run(name, func(t *testing.T) {
			var calls int
			client := NewClient(storedCredentials(), &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				return tc.rt(req)
			})})
			result, err := client.Send(context.Background(), "42", "test")
			require.Equal(t, Unknown, result.Outcome)
			require.ErrorIs(t, err, ErrUncertain)
			require.ErrorIs(t, err, tc.want)
			require.Equal(t, 1, calls) // Never retry an uncertain request.
			safe(t, result, err)
		})
	}
}
