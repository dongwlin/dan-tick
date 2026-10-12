package account

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/dongwlin/dan-tick/internal/credential"
)

const passportBaseURL = "https://passport.bilibili.com/x/passport-login/web"

var (
	ErrPassportUnavailable     = errors.New("account authentication service unavailable")
	ErrPassportInvalidResponse = errors.New("invalid account authentication response")
	ErrPassportRejected        = errors.New("account authentication request rejected")
	ErrPassportExpired         = errors.New("account authentication expired")
	// The refresh POST may have rotated the remote session. It must never be
	// replayed automatically or treated as permission to restore old credentials.
	ErrRefreshUncertain = errors.New("account refresh outcome uncertain; sign in again")
	refreshCSRFPattern  = regexp.MustCompile(`(?is)<div\b[^>]*\s+id\s*=\s*(?:"1-name"|'1-name'|1-name)(?:\s+[^>]*)?>([^<]*)</div\s*>`)
)

// QRChallenge and QRResult remain internal protocol values. In particular,
// neither the polling key nor candidate credentials belong in a Wails result.
type QRChallenge struct {
	URL string
	Key string
}

type QRResult struct {
	State       string
	Credentials credential.Credentials
}

// PassportClient handles only HTTP protocol details. Session ownership,
// persistence, UID verification and rotation recovery belong to Session.
type PassportClient struct {
	httpClient    *http.Client
	baseURL       string
	correspondURL string
}

func NewPassportClient(httpClient *http.Client) *PassportClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: requestTimeout}
	}
	safeClient := *httpClient
	safeClient.Jar = nil
	safeClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &PassportClient{
		httpClient:    &safeClient,
		baseURL:       passportBaseURL,
		correspondURL: "https://www.bilibili.com/correspond/1/",
	}
}

func (c *PassportClient) Generate(ctx context.Context) (QRChallenge, error) {
	_, body, err := c.request(ctx, http.MethodGet, c.baseURL+"/qrcode/generate", nil, nil)
	if err != nil {
		return QRChallenge{}, err
	}
	var data struct {
		URL string `json:"url"`
		Key string `json:"qrcode_key"`
	}
	if err := decodePassport(body, &data); err != nil {
		return QRChallenge{}, err
	}
	u, err := url.Parse(data.URL)
	if err != nil || !passportURL(u, "passport.bilibili.com") ||
		(u.Path != "/qrcode/h5/login" && u.Path != "/h5-app/passport/login/scan") ||
		!validPassportValue(data.Key, 512) {
		return QRChallenge{}, ErrPassportInvalidResponse
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil || len(query["qrcode_key"]) != 1 || query.Get("qrcode_key") != data.Key {
		return QRChallenge{}, ErrPassportInvalidResponse
	}
	return QRChallenge{URL: u.String(), Key: data.Key}, nil
}

func (c *PassportClient) Poll(ctx context.Context, key string) (QRResult, error) {
	if !validPassportValue(key, 512) {
		return QRResult{}, ErrPassportInvalidResponse
	}
	endpoint := c.baseURL + "/qrcode/poll?" + url.Values{"qrcode_key": {key}}.Encode()
	resp, body, err := c.request(ctx, http.MethodGet, endpoint, nil, nil)
	if err != nil {
		return QRResult{}, err
	}
	var data struct {
		Code         *int   `json:"code"`
		URL          string `json:"url"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodePassport(body, &data); err != nil {
		return QRResult{}, err
	}
	if data.Code == nil {
		return QRResult{}, ErrPassportInvalidResponse
	}
	switch *data.Code {
	case 86101:
		return QRResult{State: "waiting"}, nil
	case 86090:
		return QRResult{State: "scanned"}, nil
	case 86038:
		return QRResult{State: "expired"}, nil
	case 0:
		creds, err := passportCredentials(resp, data.RefreshToken)
		if err != nil {
			creds, err = c.crossDomainCredentials(ctx, data.URL, data.RefreshToken)
		}
		if err != nil {
			return QRResult{}, err
		}
		return QRResult{State: "confirmed", Credentials: creds}, nil
	default:
		return QRResult{}, ErrPassportRejected
	}
}

func (c *PassportClient) RefreshNeeded(ctx context.Context, creds credential.Credentials) (bool, int64, error) {
	endpoint := c.baseURL + "/cookie/info?" + url.Values{"csrf": {creds.BiliJCT}}.Encode()
	_, body, err := c.request(ctx, http.MethodGet, endpoint, nil, &creds)
	if err != nil {
		return false, 0, err
	}
	var data struct {
		Refresh   *bool `json:"refresh"`
		Timestamp int64 `json:"timestamp"`
	}
	if err := decodePassport(body, &data); err != nil {
		return false, 0, err
	}
	if data.Refresh == nil || data.Timestamp <= 0 {
		return false, 0, ErrPassportInvalidResponse
	}
	return *data.Refresh, data.Timestamp, nil
}

func (c *PassportClient) Refresh(ctx context.Context, creds credential.Credentials, timestamp int64) (credential.Credentials, error) {
	if !validPassportToken(creds.RefreshToken) {
		return credential.Credentials{}, ErrPassportInvalidResponse
	}
	path, err := correspondPath(timestamp)
	if err != nil {
		return credential.Credentials{}, err
	}
	_, body, err := c.request(ctx, http.MethodGet, c.correspondURL+path, nil, &creds)
	if err != nil {
		return credential.Credentials{}, err
	}
	matches := refreshCSRFPattern.FindAllSubmatch(body, -1)
	if len(matches) != 1 {
		return credential.Credentials{}, ErrPassportInvalidResponse
	}
	refreshCSRF := strings.TrimSpace(html.UnescapeString(string(matches[0][1])))
	if !validPassportValue(refreshCSRF, 512) {
		return credential.Credentials{}, ErrPassportInvalidResponse
	}
	form := url.Values{
		"csrf":          {creds.BiliJCT},
		"refresh_csrf":  {refreshCSRF},
		"source":        {"main_web"},
		"refresh_token": {creds.RefreshToken},
	}
	resp, body, err := c.request(ctx, http.MethodPost, c.baseURL+"/cookie/refresh", form, &creds)
	if err != nil {
		return credential.Credentials{}, errors.Join(ErrRefreshUncertain, err)
	}
	var data struct {
		Status       *int   `json:"status"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := decodePassport(body, &data); err != nil {
		if errors.Is(err, ErrPassportInvalidResponse) {
			err = errors.Join(ErrRefreshUncertain, err)
		}
		return credential.Credentials{}, err
	}
	if data.Status == nil || *data.Status != 0 {
		return credential.Credentials{}, ErrRefreshUncertain
	}
	updated, err := passportCredentials(resp, data.RefreshToken)
	if err != nil {
		return credential.Credentials{}, errors.Join(ErrRefreshUncertain, err)
	}
	return updated, nil
}

func (c *PassportClient) Confirm(ctx context.Context, creds credential.Credentials, oldRefreshToken string) error {
	if !validPassportToken(oldRefreshToken) {
		return ErrPassportInvalidResponse
	}
	form := url.Values{"csrf": {creds.BiliJCT}, "refresh_token": {oldRefreshToken}}
	_, body, err := c.request(ctx, http.MethodPost, c.baseURL+"/confirm/refresh", form, &creds)
	if err != nil {
		return err
	}
	return decodePassport(body, nil)
}

// A reported ticket flow is deliberately a single allowlisted request rather
// than general redirect or SSO support. No existing Cookie is sent to biligame.
func (c *PassportClient) crossDomainCredentials(ctx context.Context, rawURL, refreshToken string) (credential.Credentials, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !passportURL(u, "passport.biligame.com") {
		return credential.Credentials{}, ErrPassportInvalidResponse
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return credential.Credentials{}, ErrPassportInvalidResponse
	}
	if u.Path == "/crossDomain" {
		resp := &http.Response{Header: make(http.Header)}
		for _, name := range []string{"SESSDATA", "bili_jct", "DedeUserID"} {
			if len(query[name]) != 1 {
				return credential.Credentials{}, ErrPassportInvalidResponse
			}
			cookie := &http.Cookie{Name: name, Value: query.Get(name)}
			if cookie.Valid() != nil {
				return credential.Credentials{}, ErrPassportInvalidResponse
			}
			resp.Header.Add("Set-Cookie", cookie.String())
		}
		return passportCredentials(resp, refreshToken)
	}
	if u.Path != "/x/passport-login/web/crossDomain" || len(query) != 1 ||
		len(query["ticket"]) != 1 || !validPassportValue(query.Get("ticket"), 2048) {
		return credential.Credentials{}, ErrPassportInvalidResponse
	}
	resp, _, err := c.requestStatus(ctx, http.MethodGet, u.String(), nil, nil, true)
	if err != nil {
		return credential.Credentials{}, err
	}
	return passportCredentials(resp, refreshToken)
}

func (c *PassportClient) request(ctx context.Context, method, endpoint string, form url.Values, creds *credential.Credentials) (*http.Response, []byte, error) {
	return c.requestStatus(ctx, method, endpoint, form, creds, false)
}

func (c *PassportClient) requestStatus(ctx context.Context, method, endpoint string, form url.Values, creds *credential.Credentials, ticket bool) (*http.Response, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var reader io.Reader
	if form != nil {
		reader = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, nil, ErrPassportInvalidResponse
	}
	req.Header.Set("Accept", "application/json, text/html")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://passport.bilibili.com/")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if creds != nil {
		for _, cookie := range []*http.Cookie{
			{Name: "SESSDATA", Value: creds.SESSDATA},
			{Name: "bili_jct", Value: creds.BiliJCT},
			{Name: "DedeUserID", Value: creds.UID},
		} {
			if !validPassportValue(cookie.Value, 2048) || cookie.Valid() != nil {
				return nil, nil, ErrPassportInvalidResponse
			}
			req.AddCookie(cookie)
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, errors.Join(ErrPassportUnavailable, ctx.Err())
		}
		return nil, nil, ErrPassportUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && !(ticket && resp.StatusCode == http.StatusFound) {
		return nil, nil, ErrPassportUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil || len(body) > maxResponseSize {
		return nil, nil, ErrPassportInvalidResponse
	}
	return resp, body, nil
}

func decodePassport(body []byte, data any) error {
	var response struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(body, &response) != nil || response.Code == nil {
		return ErrPassportInvalidResponse
	}
	switch *response.Code {
	case 0:
	case -101, -111, 86095:
		return ErrPassportExpired
	default:
		return ErrPassportRejected
	}
	if data != nil && (len(response.Data) == 0 || string(response.Data) == "null" || json.Unmarshal(response.Data, data) != nil) {
		return ErrPassportInvalidResponse
	}
	return nil
}

func passportCredentials(resp *http.Response, refreshToken string) (credential.Credentials, error) {
	creds := credential.Credentials{Version: 1, Source: "qr", RefreshState: "ready", RefreshToken: refreshToken}
	if !validPassportToken(refreshToken) {
		return credential.Credentials{}, ErrPassportInvalidResponse
	}
	seen := make(map[string]bool)
	for _, cookie := range resp.Cookies() {
		var target *string
		switch cookie.Name {
		case "SESSDATA":
			target = &creds.SESSDATA
		case "bili_jct":
			target = &creds.BiliJCT
		case "DedeUserID":
			target = &creds.UID
		default:
			continue
		}
		if seen[cookie.Name] || cookie.MaxAge < 0 || !validPassportValue(cookie.Value, 2048) || cookie.Valid() != nil {
			return credential.Credentials{}, ErrPassportInvalidResponse
		}
		seen[cookie.Name] = true
		*target = cookie.Value
	}
	uid, err := strconv.ParseUint(creds.UID, 10, 64)
	if !seen["SESSDATA"] || !seen["bili_jct"] || !seen["DedeUserID"] || err != nil || uid == 0 ||
		strconv.FormatUint(uid, 10) != creds.UID {
		return credential.Credentials{}, ErrPassportInvalidResponse
	}
	return creds, nil
}

func passportURL(u *url.URL, host string) bool {
	return u != nil && u.Scheme == "https" && u.Host == host && u.User == nil && u.Fragment == "" && u.RawPath == ""
}

func validPassportValue(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r <= 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

// Keep refresh tokens compatible with the single-item credential vault format.
// In particular, reject Unicode and cookie separators before accepting a QR
// candidate or beginning a refresh side effect.
func validPassportToken(value string) bool {
	return validPassportValue(value, 2048) && (&http.Cookie{Name: "refresh_token", Value: value}).Valid() == nil
}

func correspondPath(timestamp int64) (string, error) {
	if timestamp <= 0 {
		return "", ErrPassportInvalidResponse
	}
	const publicKeyPEM = `-----BEGIN PUBLIC KEY-----
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDLgd2OAkcGVtoE3ThUREbio0Eg
Uc/prcajMKXvkCKFCWhJYJcLkcM2DKKcSeFpD/j6Boy538YXnR6VhcuUJOhH2x71
nzPjfdTcqMz7djHum0qSZA0AyCBDABUqCrfNgCiJ00Ra7GmRj+YCK1NJEuewlb40
JNrRuoEUXpabUzGB8QIDAQAB
-----END PUBLIC KEY-----`
	block, _ := pem.Decode([]byte(publicKeyPEM))
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", ErrPassportInvalidResponse
	}
	publicKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return "", ErrPassportInvalidResponse
	}
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, []byte("refresh_"+strconv.FormatInt(timestamp, 10)), nil)
	if err != nil {
		return "", ErrPassportUnavailable
	}
	return hex.EncodeToString(ciphertext), nil
}
