package account

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
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

const passportRefreshSecret = "fake_refresh_sensitive_value"

func passportTestCredentials() credential.Credentials {
	return credential.Credentials{
		Version: 1, Source: "qr", UID: "42", RefreshState: "ready",
		SESSDATA: sessionSecret, BiliJCT: csrfSecret, RefreshToken: passportRefreshSecret,
	}
}

func passportTestClient(server *httptest.Server) *PassportClient {
	client := NewPassportClient(server.Client())
	client.baseURL = server.URL
	client.correspondURL = server.URL + "/correspond/1/"
	return client
}

func passportWriteCookies(w http.ResponseWriter, uid string) {
	for name, value := range map[string]string{"SESSDATA": sessionSecret, "bili_jct": csrfSecret, "DedeUserID": uid} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/"})
	}
}

func assertPassportErrorSafe(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	for _, secret := range []string{sessionSecret, csrfSecret, passportRefreshSecret} {
		require.NotContains(t, err.Error(), secret)
	}
}

func TestPassportGenerateValidatesQRURLAndKey(t *testing.T) {
	for name, rawURL := range map[string]string{
		"current path": "https://passport.bilibili.com/qrcode/h5/login?qrcode_key=test-key",
		"legacy path":  "https://passport.bilibili.com/h5-app/passport/login/scan?navhide=1&qrcode_key=test-key&from=",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/qrcode/generate", r.URL.Path)
				require.Equal(t, http.MethodGet, r.Method)
				require.Empty(t, r.Cookies())
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]string{"url": rawURL, "qrcode_key": "test-key"}})
			}))
			defer server.Close()
			challenge, err := passportTestClient(server).Generate(context.Background())
			require.NoError(t, err)
			require.Equal(t, QRChallenge{URL: rawURL, Key: "test-key"}, challenge)
		})
	}
	for name, data := range map[string]map[string]string{
		"http":            {"url": "http://passport.bilibili.com/qrcode/h5/login?qrcode_key=test-key", "qrcode_key": "test-key"},
		"other host":      {"url": "https://attacker.example/qrcode/h5/login?qrcode_key=test-key", "qrcode_key": "test-key"},
		"userinfo":        {"url": "https://secret@passport.bilibili.com/qrcode/h5/login?qrcode_key=test-key", "qrcode_key": "test-key"},
		"explicit port":   {"url": "https://passport.bilibili.com:443/qrcode/h5/login?qrcode_key=test-key", "qrcode_key": "test-key"},
		"fragment":        {"url": "https://passport.bilibili.com/qrcode/h5/login?qrcode_key=test-key#secret", "qrcode_key": "test-key"},
		"unexpected path": {"url": "https://passport.bilibili.com/other?qrcode_key=test-key", "qrcode_key": "test-key"},
		"encoded path":    {"url": "https://passport.bilibili.com/qrcode/h5/%6cogin?qrcode_key=test-key", "qrcode_key": "test-key"},
		"wrong key":       {"url": "https://passport.bilibili.com/qrcode/h5/login?qrcode_key=other-key", "qrcode_key": "test-key"},
		"duplicate key":   {"url": "https://passport.bilibili.com/qrcode/h5/login?qrcode_key=test-key&qrcode_key=test-key", "qrcode_key": "test-key"},
		"missing key":     {"url": "https://passport.bilibili.com/qrcode/h5/login"},
		"oversize key":    {"url": "https://passport.bilibili.com/qrcode/h5/login", "qrcode_key": strings.Repeat("x", 513)},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
			}))
			defer server.Close()
			result, err := passportTestClient(server).Generate(context.Background())
			require.Empty(t, result)
			require.ErrorIs(t, err, ErrPassportInvalidResponse)
		})
	}
}

func TestPassportProtocolRejectsHTTPAndInvalidBusinessResponses(t *testing.T) {
	for name, test := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"HTTP failure":     {http.StatusBadGateway, sessionSecret, ErrPassportUnavailable},
		"business expired": {http.StatusOK, `{"code":-101,"message":"` + sessionSecret + `"}`, ErrPassportExpired},
		"csrf expired":     {http.StatusOK, `{"code":-111}`, ErrPassportExpired},
		"token mismatch":   {http.StatusOK, `{"code":86095}`, ErrPassportExpired},
		"business refused": {http.StatusOK, `{"code":-400,"message":"` + csrfSecret + `"}`, ErrPassportRejected},
		"no code":          {http.StatusOK, `{"data":{}}`, ErrPassportInvalidResponse},
		"null code":        {http.StatusOK, `{"code":null}`, ErrPassportInvalidResponse},
		"missing data":     {http.StatusOK, `{"code":0}`, ErrPassportInvalidResponse},
		"null data":        {http.StatusOK, `{"code":0,"data":null}`, ErrPassportInvalidResponse},
		"wrong data":       {http.StatusOK, `{"code":0,"data":false}`, ErrPassportInvalidResponse},
		"malformed":        {http.StatusOK, `{"code":0`, ErrPassportInvalidResponse},
		"oversize":         {http.StatusOK, strings.Repeat("x", maxResponseSize+1), ErrPassportInvalidResponse},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			_, err := passportTestClient(server).Generate(context.Background())
			require.ErrorIs(t, err, test.want)
			assertPassportErrorSafe(t, err)
		})
	}
}

func TestPassportPollStatesAndDirectCredentials(t *testing.T) {
	for name, test := range map[string]struct {
		code int
		want string
		err  error
	}{
		"waiting":   {86101, "waiting", nil},
		"scanned":   {86090, "scanned", nil},
		"expired":   {86038, "expired", nil},
		"confirmed": {0, "confirmed", nil},
		"unknown":   {1, "", ErrPassportRejected},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/qrcode/poll", r.URL.Path)
				require.Equal(t, "test-key", r.URL.Query().Get("qrcode_key"))
				require.Empty(t, r.Cookies())
				passportWriteCookies(w, "42")
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"code": test.code, "refresh_token": passportRefreshSecret, "message": sessionSecret}})
			}))
			defer server.Close()
			result, err := passportTestClient(server).Poll(context.Background(), "test-key")
			if test.err != nil {
				require.ErrorIs(t, err, test.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.want, result.State)
			if test.want == "confirmed" {
				require.Equal(t, passportTestCredentials(), result.Credentials)
			} else {
				require.Empty(t, result.Credentials)
			}
			assertPassportErrorSafe(t, err)
		})
	}
}

func TestPassportPollRejectsIncompleteCredentials(t *testing.T) {
	for name, test := range map[string]struct {
		uid, token string
		cookies    []string
		body       string
	}{
		"missing session": {uid: "42", token: passportRefreshSecret, cookies: []string{"bili_jct=" + csrfSecret, "DedeUserID=42"}},
		"missing csrf":    {uid: "42", token: passportRefreshSecret, cookies: []string{"SESSDATA=" + sessionSecret, "DedeUserID=42"}},
		"invalid UID":     {uid: "not-a-uid", token: passportRefreshSecret},
		"zero UID":        {uid: "0", token: passportRefreshSecret},
		"missing token":   {uid: "42"},
		"unicode token":   {uid: "42", token: "token汉字"},
		"separator token": {uid: "42", token: "token;other"},
		"duplicate csrf":  {uid: "42", token: passportRefreshSecret, cookies: []string{"SESSDATA=" + sessionSecret, "bili_jct=" + csrfSecret, "bili_jct=other", "DedeUserID=42"}},
		"deleted cookie":  {uid: "42", token: passportRefreshSecret, cookies: []string{"SESSDATA=" + sessionSecret + "; Max-Age=0", "bili_jct=" + csrfSecret, "DedeUserID=42"}},
		"missing state":   {uid: "42", token: passportRefreshSecret, body: `{"code":0,"data":{}}`},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.cookies == nil {
					passportWriteCookies(w, test.uid)
				} else {
					for _, value := range test.cookies {
						w.Header().Add("Set-Cookie", value)
					}
				}
				if test.body != "" {
					_, _ = io.WriteString(w, test.body)
				} else {
					_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"code": 0, "refresh_token": test.token}})
				}
			}))
			defer server.Close()
			result, err := passportTestClient(server).Poll(context.Background(), "test-key")
			require.Empty(t, result)
			require.ErrorIs(t, err, ErrPassportInvalidResponse)
			assertPassportErrorSafe(t, err)
		})
	}
}

// Rewrite only test traffic to an httptest server, while retaining the real
// allowlist checks. No test can contact a production Bilibili endpoint.
type passportTestTransport struct {
	server *url.URL
	base   http.RoundTripper
}

func (t passportTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	copy.URL.Scheme = t.server.Scheme
	copy.URL.Host = t.server.Host
	return t.base.RoundTrip(copy)
}

func passportRoutedClient(t *testing.T, server *httptest.Server) *PassportClient {
	t.Helper()
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := NewPassportClient(&http.Client{Transport: passportTestTransport{server: u, base: server.Client().Transport}})
	return client
}

func TestPassportPollTicketGetsCookiesWithoutFollowingRedirectOrSendingCookies(t *testing.T) {
	var ticketRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/x/passport-login/web/qrcode/poll" {
			_, _ = io.WriteString(w, `{"code":0,"data":{"code":0,"refresh_token":"`+passportRefreshSecret+`","url":"https://passport.biligame.com/x/passport-login/web/crossDomain?ticket=fake-ticket"}}`)
			return
		}
		require.Equal(t, "/x/passport-login/web/crossDomain", r.URL.Path)
		require.Equal(t, "fake-ticket", r.URL.Query().Get("ticket"))
		require.Empty(t, r.Cookies())
		require.Contains(t, r.UserAgent(), "Mozilla/5.0")
		require.Equal(t, "https://passport.bilibili.com/", r.Referer())
		ticketRequests.Add(1)
		passportWriteCookies(w, "42")
		w.Header().Set("Location", "https://attacker.example/collect")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()
	result, err := passportRoutedClient(t, server).Poll(context.Background(), "test-key")
	require.NoError(t, err)
	require.Equal(t, QRResult{State: "confirmed", Credentials: passportTestCredentials()}, result)
	require.EqualValues(t, 1, ticketRequests.Load())
}

func TestPassportPollTicketRejectsUnsafeURL(t *testing.T) {
	for name, ticketURL := range map[string]string{
		"other host":       "https://attacker.example/x/passport-login/web/crossDomain?ticket=fake-ticket",
		"HTTP":             "http://passport.biligame.com/x/passport-login/web/crossDomain?ticket=fake-ticket",
		"userinfo":         "https://user@passport.biligame.com/x/passport-login/web/crossDomain?ticket=fake-ticket",
		"port":             "https://passport.biligame.com:443/x/passport-login/web/crossDomain?ticket=fake-ticket",
		"fragment":         "https://passport.biligame.com/x/passport-login/web/crossDomain?ticket=fake-ticket#extra",
		"other path":       "https://passport.biligame.com/arbitrary?ticket=fake-ticket",
		"missing ticket":   "https://passport.biligame.com/x/passport-login/web/crossDomain",
		"duplicate ticket": "https://passport.biligame.com/x/passport-login/web/crossDomain?ticket=first&ticket=second",
		"extra parameter":  "https://passport.biligame.com/x/passport-login/web/crossDomain?ticket=fake-ticket&next=other",
		"oversize ticket":  "https://passport.biligame.com/x/passport-login/web/crossDomain?ticket=" + strings.Repeat("x", 2049),
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"code": 0, "refresh_token": passportRefreshSecret, "url": ticketURL}})
			}))
			defer server.Close()
			result, err := passportRoutedClient(t, server).Poll(context.Background(), "test-key")
			require.Empty(t, result)
			require.ErrorIs(t, err, ErrPassportInvalidResponse)
			require.EqualValues(t, 1, requests.Load())
			assertPassportErrorSafe(t, err)
		})
	}
}

func TestPassportPollLegacyCrossDomainDoesNotFetchURL(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		legacyURL := "https://passport.biligame.com/crossDomain?" + url.Values{
			"SESSDATA": {sessionSecret}, "bili_jct": {csrfSecret}, "DedeUserID": {"42"}, "gourl": {"https://passport.bilibili.com"},
		}.Encode()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"code": 0, "refresh_token": passportRefreshSecret, "url": legacyURL}})
	}))
	defer server.Close()
	result, err := passportRoutedClient(t, server).Poll(context.Background(), "test-key")
	require.NoError(t, err)
	require.Equal(t, passportTestCredentials(), result.Credentials)
	require.EqualValues(t, 1, requests.Load())
}

func TestPassportRefreshInfoValidatesFieldsAndUsesMatchingCookie(t *testing.T) {
	for name, body := range map[string]string{
		"needed":     `{"code":0,"data":{"refresh":true,"timestamp":1800000000000}}`,
		"not needed": `{"code":0,"data":{"refresh":false,"timestamp":1800000000000}}`,
		"no flag":    `{"code":0,"data":{"timestamp":1800000000000}}`,
		"no time":    `{"code":0,"data":{"refresh":false}}`,
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/cookie/info", r.URL.Path)
				require.Equal(t, csrfSecret, r.URL.Query().Get("csrf"))
				passportAssertCookies(t, r, passportTestCredentials())
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			needed, timestamp, err := passportTestClient(server).RefreshNeeded(context.Background(), passportTestCredentials())
			if name == "needed" || name == "not needed" {
				require.NoError(t, err)
				require.Equal(t, name == "needed", needed)
				require.EqualValues(t, 1800000000000, timestamp)
			} else {
				require.ErrorIs(t, err, ErrPassportInvalidResponse)
				require.False(t, needed)
				require.Zero(t, timestamp)
			}
		})
	}
}

func passportAssertCookies(t *testing.T, r *http.Request, creds credential.Credentials) {
	t.Helper()
	require.Len(t, r.Cookies(), 3)
	for name, value := range map[string]string{"SESSDATA": creds.SESSDATA, "bili_jct": creds.BiliJCT, "DedeUserID": creds.UID} {
		cookie, err := r.Cookie(name)
		require.NoError(t, err)
		require.Equal(t, value, cookie.Value)
	}
	require.NotContains(t, r.Header.Get("Cookie"), creds.RefreshToken)
}

func TestPassportRefreshAndConfirmUseNewCookiesAndOldToken(t *testing.T) {
	var requests atomic.Int32
	old := passportTestCredentials()
	updated := old
	updated.SESSDATA, updated.BiliJCT, updated.RefreshToken = "new-session", "new-csrf", "new-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch {
		case strings.HasPrefix(r.URL.Path, "/correspond/1/"):
			ciphertext, err := hex.DecodeString(strings.TrimPrefix(r.URL.Path, "/correspond/1/"))
			require.NoError(t, err)
			require.Len(t, ciphertext, 128)
			passportAssertCookies(t, r, old)
			_, _ = io.WriteString(w, `<html><div class="token" id='1-name'> refresh-csrf </div></html>`)
		case r.URL.Path == "/cookie/refresh":
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "application/x-www-form-urlencoded", r.Header.Get("Content-Type"))
			passportAssertCookies(t, r, old)
			require.NoError(t, r.ParseForm())
			require.Equal(t, url.Values{"csrf": {old.BiliJCT}, "refresh_csrf": {"refresh-csrf"}, "source": {"main_web"}, "refresh_token": {old.RefreshToken}}, r.PostForm)
			http.SetCookie(w, &http.Cookie{Name: "SESSDATA", Value: updated.SESSDATA})
			http.SetCookie(w, &http.Cookie{Name: "bili_jct", Value: updated.BiliJCT})
			http.SetCookie(w, &http.Cookie{Name: "DedeUserID", Value: updated.UID})
			_, _ = io.WriteString(w, `{"code":0,"data":{"status":0,"refresh_token":"new-token"}}`)
		case r.URL.Path == "/confirm/refresh":
			passportAssertCookies(t, r, updated)
			require.NoError(t, r.ParseForm())
			require.Equal(t, url.Values{"csrf": {updated.BiliJCT}, "refresh_token": {old.RefreshToken}}, r.PostForm)
			_, _ = io.WriteString(w, `{"code":0}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	client := passportTestClient(server)
	result, err := client.Refresh(context.Background(), old, 1800000000000)
	require.NoError(t, err)
	require.Equal(t, updated, result)
	require.NoError(t, client.Confirm(context.Background(), result, old.RefreshToken))
	require.EqualValues(t, 3, requests.Load())
}

func TestPassportRefreshDoesNotPostWhenCSRFIsInvalid(t *testing.T) {
	for name, htmlBody := range map[string]string{
		"missing":    `<html></html>`,
		"duplicate":  `<div id="1-name">first</div><div id="1-name">second</div>`,
		"empty":      `<div id="1-name"> </div>`,
		"markup":     `<div id="1-name"><span>secret</span></div>`,
		"wrong attr": `<div data-id="1-name">secret</div>`,
		"wrong id":   `<div id=1-name-extra>secret</div>`,
		"oversize":   `<div id="1-name">` + strings.Repeat("x", 513) + `</div>`,
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				_, _ = io.WriteString(w, htmlBody)
			}))
			defer server.Close()
			result, err := passportTestClient(server).Refresh(context.Background(), passportTestCredentials(), 1800000000000)
			require.Empty(t, result)
			require.ErrorIs(t, err, ErrPassportInvalidResponse)
			require.NotErrorIs(t, err, ErrRefreshUncertain)
			require.EqualValues(t, 1, requests.Load())
		})
	}
}

func TestPassportRefreshRejectsRemoteFailuresAndClassifiesUncertainSuccess(t *testing.T) {
	for name, test := range map[string]struct {
		status int
		body   string
		want   error
	}{
		"expired":        {http.StatusOK, `{"code":-101,"message":"` + sessionSecret + `"}`, ErrPassportExpired},
		"csrf rejected":  {http.StatusOK, `{"code":-111}`, ErrPassportExpired},
		"token rejected": {http.StatusOK, `{"code":86095}`, ErrPassportExpired},
		"other rejected": {http.StatusOK, `{"code":-400}`, ErrPassportRejected},
		"HTTP uncertain": {http.StatusBadGateway, sessionSecret, ErrRefreshUncertain},
		"JSON uncertain": {http.StatusOK, `{"code":0`, ErrRefreshUncertain},
		"missing status": {http.StatusOK, `{"code":0,"data":{"refresh_token":"new-token"}}`, ErrRefreshUncertain},
		"failed status":  {http.StatusOK, `{"code":0,"data":{"status":1,"refresh_token":"new-token"}}`, ErrRefreshUncertain},
		"missing cookie": {http.StatusOK, `{"code":0,"data":{"status":0,"refresh_token":"new-token"}}`, ErrRefreshUncertain},
	} {
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if strings.HasPrefix(r.URL.Path, "/correspond/1/") {
					_, _ = io.WriteString(w, `<div id="1-name">refresh-csrf</div>`)
					return
				}
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			result, err := passportTestClient(server).Refresh(context.Background(), passportTestCredentials(), 1800000000000)
			require.Empty(t, result)
			require.ErrorIs(t, err, test.want)
			require.EqualValues(t, 2, requests.Load())
			assertPassportErrorSafe(t, err)
		})
	}
}

func TestPassportNeverFollowsRedirectOrUsesInjectedJar(t *testing.T) {
	var forwarded atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded.Add(1)
	}))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		passportAssertCookies(t, r, passportTestCredentials())
		http.Redirect(w, r, other.URL+"/collect", http.StatusFound)
	}))
	defer server.Close()
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	jar.SetCookies(u, []*http.Cookie{{Name: "unrelated", Value: "secret"}})
	injected := &http.Client{Jar: jar}
	client := NewPassportClient(injected)
	client.baseURL = server.URL
	_, _, err = client.RefreshNeeded(context.Background(), passportTestCredentials())
	require.ErrorIs(t, err, ErrPassportUnavailable)
	require.Zero(t, forwarded.Load())
	require.Same(t, jar, injected.Jar)
}

func TestPassportCancellationTimeoutAndTransportErrorsAreSafe(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	client := passportTestClient(server)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Generate(cancelled)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, ErrPassportUnavailable)
	timed, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer stop()
	_, err = client.Generate(timed)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorIs(t, err, ErrPassportUnavailable)

	client = NewPassportClient(&http.Client{Transport: leakingTransport{}})
	_, err = client.Poll(context.Background(), "test-key")
	require.ErrorIs(t, err, ErrPassportUnavailable)
	assertPassportErrorSafe(t, err)
}

func TestPassportRefreshInterruptedAfterPostIsUncertain(t *testing.T) {
	posted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/correspond/1/") {
			_, _ = io.WriteString(w, `<div id="1-name">refresh-csrf</div>`)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		close(posted)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := passportTestClient(server).Refresh(ctx, passportTestCredentials(), 1800000000000)
		done <- err
	}()
	<-posted
	cancel()
	err := <-done
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, ErrRefreshUncertain)
	assertPassportErrorSafe(t, err)
}

func TestPassportInvalidInputsDoNotUseNetwork(t *testing.T) {
	client := NewPassportClient(&http.Client{Transport: leakingTransport{}})
	_, err := client.Poll(context.Background(), "bad\nkey")
	require.ErrorIs(t, err, ErrPassportInvalidResponse)
	_, err = client.Refresh(context.Background(), passportTestCredentials(), 0)
	require.ErrorIs(t, err, ErrPassportInvalidResponse)
	err = client.Confirm(context.Background(), passportTestCredentials(), "")
	require.ErrorIs(t, err, ErrPassportInvalidResponse)
	bad := passportTestCredentials()
	bad.RefreshToken = "bad;token"
	_, err = client.Refresh(context.Background(), bad, 1800000000000)
	require.ErrorIs(t, err, ErrPassportInvalidResponse)
	bad = passportTestCredentials()
	bad.SESSDATA = "bad;session"
	_, _, err = client.RefreshNeeded(context.Background(), bad)
	require.ErrorIs(t, err, ErrPassportInvalidResponse)
	require.False(t, errors.Is(err, ErrPassportUnavailable))
}
