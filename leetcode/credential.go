package leetcode

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/j178/kooky"
	_ "github.com/j178/kooky/browser/brave"
	_ "github.com/j178/kooky/browser/chrome"
	_ "github.com/j178/kooky/browser/edge"
	_ "github.com/j178/kooky/browser/firefox"
	_ "github.com/j178/kooky/browser/safari"

	"github.com/j178/leetgo/config"
)

type CredentialsProvider interface {
	Source() string
	AddCredentials(req *http.Request) error
}

type ResettableProvider interface {
	Reset()
}

type NeedClient interface {
	SetClient(c Client)
}

type nonAuth struct{}

func NonAuth() CredentialsProvider {
	return &nonAuth{}
}

func (n *nonAuth) Source() string {
	return "none"
}

func (n *nonAuth) AddCredentials(req *http.Request) error {
	return errors.New("no credentials provided")
}

func (n *nonAuth) Reset() {}

type cookiesAuth struct {
	LeetCodeSession string
	CsrfToken       string
	CfClearance     string // Cloudflare cookie, US only
}

func NewCookiesAuth(session, csrftoken, cfClearance string) CredentialsProvider {
	return &cookiesAuth{LeetCodeSession: session, CsrfToken: csrftoken, CfClearance: cfClearance}
}

func (c *cookiesAuth) Source() string {
	return "cookies"
}

func (c *cookiesAuth) AddCredentials(req *http.Request) error {
	if !c.hasAuth() {
		return errors.New("cookies not found")
	}
	req.AddCookie(&http.Cookie{Name: "LEETCODE_SESSION", Value: c.LeetCodeSession})
	req.AddCookie(&http.Cookie{Name: "csrftoken", Value: c.CsrfToken})
	req.AddCookie(&http.Cookie{Name: "cf_clearance", Value: c.CfClearance})

	req.Header.Add("x-csrftoken", c.CsrfToken)
	return nil
}

func (c *cookiesAuth) Reset() {}

func (c *cookiesAuth) hasAuth() bool {
	return c.LeetCodeSession != "" && c.CsrfToken != ""
}

type passwordAuth struct {
	cookiesAuth
	mu       sync.Mutex
	c        Client
	username string
	password string
}

func NewPasswordAuth(username, passwd string) CredentialsProvider {
	return &passwordAuth{username: username, password: passwd}
}

func (p *passwordAuth) Source() string {
	return "password"
}

func (p *passwordAuth) SetClient(c Client) {
	p.c = c
}

func (p *passwordAuth) AddCredentials(req *http.Request) error {
	if p.username == "" || p.password == "" {
		return errors.New("username or password is empty")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.hasAuth() {
		log.Info("logging in with username and password")
		resp, err := p.c.Login(p.username, p.password)
		if err != nil {
			return err
		}
		cookies := resp.Cookies()
		for _, cookie := range cookies {
			if cookie.Name == "LEETCODE_SESSION" {
				p.LeetCodeSession = cookie.Value
			}
			if cookie.Name == "csrftoken" {
				p.CsrfToken = cookie.Value
			}
		}
		if !p.hasAuth() {
			return errors.New("login failed")
		}

		// Cache cookies to user cache directory
		site := p.c.BaseURI()
		if err := saveCookiesToCache(p.LeetCodeSession, p.CsrfToken, p.CfClearance, site); err != nil {
			log.Warn("failed to cache cookies", "error", err)
		} else {
			log.Debug("cached cookies to user cache directory")
		}
	}
	return p.cookiesAuth.AddCredentials(req)
}

func (p *passwordAuth) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.LeetCodeSession = ""
	p.CsrfToken = ""
	p.CfClearance = ""

	// Clear cached cookies
	clearCookiesFromCache()
}

type cacheAuth struct {
	cookiesAuth
	mu   sync.Mutex
	c    Client
	site string
}

func NewCacheAuth(site string) CredentialsProvider {
	return &cacheAuth{site: site}
}

func (ca *cacheAuth) Source() string {
	return "cache"
}

func (ca *cacheAuth) SetClient(c Client) {
	ca.c = c
}

func (ca *cacheAuth) AddCredentials(req *http.Request) error {
	ca.mu.Lock()
	defer ca.mu.Unlock()

	if !ca.hasAuth() {
		// Load from cache
		session, csrf, cf, err := loadCookiesFromCache(ca.site)
		if err != nil {
			return fmt.Errorf("load from cache: %w", err)
		}
		ca.LeetCodeSession = session
		ca.CsrfToken = csrf
		ca.CfClearance = cf
		log.Debug("loaded credentials from cache")
	}

	return ca.cookiesAuth.AddCredentials(req)
}

func (ca *cacheAuth) Reset() {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.LeetCodeSession = ""
	ca.CsrfToken = ""
	ca.CfClearance = ""
	clearCookiesFromCache()
}

type browserAuth struct {
	cookiesAuth
	mu       sync.Mutex
	c        Client
	browsers []string
}

func NewBrowserAuth(browsers []string) CredentialsProvider {
	return &browserAuth{browsers: browsers}
}

func (b *browserAuth) Source() string {
	return "browser"
}

func (b *browserAuth) SetClient(c Client) {
	b.c = c
}

func (b *browserAuth) AddCredentials(req *http.Request) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var errs []error
	if !b.hasAuth() {
		u, _ := url.Parse(b.c.BaseURI())
		domain := u.Host

		defer func(start time.Time) {
			log.Debug("finished reading cookies", "elapsed", time.Since(start))
		}(time.Now())

		cookieStores := kooky.FindCookieStores(b.browsers...)
		filters := []kooky.Filter{
			kooky.DomainHasSuffix(domain),
			kooky.FilterFunc(
				func(cookie *kooky.Cookie) bool {
					return kooky.Name("LEETCODE_SESSION").Filter(cookie) ||
						kooky.Name("csrftoken").Filter(cookie) ||
						kooky.Name("cf_clearance").Filter(cookie)
				},
			),
		}

		for _, store := range cookieStores {
			log.Debug("reading cookies", "browser", store.Browser(), "file", store.FilePath())
			cookies, err := store.ReadCookies(filters...)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, cookie := range cookies {
				if cookie.Name == "LEETCODE_SESSION" {
					b.LeetCodeSession = cookie.Value
				}
				if cookie.Name == "csrftoken" {
					b.CsrfToken = cookie.Value
				}
				if cookie.Name == "cf_clearance" {
					b.CfClearance = cookie.Value
				}
			}
			if b.LeetCodeSession == "" || b.CsrfToken == "" {
				errs = append(errs, fmt.Errorf("LeetCode cookies not found in %s", store.FilePath()))
				continue
			}
			log.Info("reading leetcode cookies", "browser", store.Browser(), "domain", domain)
			break
		}

		// Cache cookies to user cache directory
		if b.hasAuth() {
			site := b.c.BaseURI()
			if err := saveCookiesToCache(b.LeetCodeSession, b.CsrfToken, b.CfClearance, site); err != nil {
				log.Warn("failed to cache cookies", "error", err)
			} else {
				log.Debug("cached cookies to user cache directory")
			}
		}
	}
	if !b.hasAuth() {
		if len(errs) > 0 {
			return fmt.Errorf("failed to read cookies: %w", errors.Join(errs...))
		}
		return errors.New("no cookies found in browsers")
	}

	return b.cookiesAuth.AddCredentials(req)
}

func (b *browserAuth) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.LeetCodeSession = ""
	b.CsrfToken = ""
	b.CfClearance = ""

	// Clear cached cookies
	clearCookiesFromCache()
}

type combinedAuth struct {
	providers []CredentialsProvider
}

func NewCombinedAuth(providers ...CredentialsProvider) CredentialsProvider {
	return &combinedAuth{providers: providers}
}

func (c *combinedAuth) Source() string {
	return "combined sources"
}

func (c *combinedAuth) AddCredentials(req *http.Request) error {
	for _, p := range c.providers {
		if err := p.AddCredentials(req); err == nil {
			return nil
		} else {
			log.Debug("read credentials from %s failed: %v", p.Source(), err)
		}
	}
	return errors.New("no credentials provided")
}

func (c *combinedAuth) SetClient(client Client) {
	for _, p := range c.providers {
		if r, ok := p.(NeedClient); ok {
			r.SetClient(client)
		}
	}
}

func (c *combinedAuth) Reset() {
	for _, p := range c.providers {
		if r, ok := p.(ResettableProvider); ok {
			r.Reset()
		}
	}
}

func ReadCredentials() CredentialsProvider {
	cfg := config.Get()
	var providers []CredentialsProvider

	// Highest priority: load from cache
	site := string(cfg.LeetCode.Site)
	providers = append(providers, NewCacheAuth(site))

	// Second priority: environment variables
	session := os.Getenv("LEETCODE_SESSION")
	csrfToken := os.Getenv("LEETCODE_CSRFTOKEN")
	if session != "" && csrfToken != "" {
		cfClearance := os.Getenv("LEETCODE_CFCLEARANCE")
		providers = append(providers, NewCookiesAuth(session, csrfToken, cfClearance))
	}

	// Then try configured credential sources
	for _, from := range cfg.LeetCode.Credentials.From {
		switch from {
		case "browser":
			providers = append(providers, NewBrowserAuth(cfg.LeetCode.Credentials.Browsers))
		case "password":
			username := os.Getenv("LEETCODE_USERNAME")
			password := os.Getenv("LEETCODE_PASSWORD")
			providers = append(providers, NewPasswordAuth(username, password))
		case "cookies":
			// Already handled above
			continue
		}
	}
	if len(providers) == 0 {
		return NonAuth()
	}
	if len(providers) == 1 {
		return providers[0]
	}
	return NewCombinedAuth(providers...)
}

// CachedCredentials represents cached authentication credentials
type CachedCredentials struct {
	LeetCodeSession string    `json:"leetcode_session"`
	CsrfToken       string    `json:"csrf_token"`
	CfClearance     string    `json:"cf_clearance,omitempty"`
	CachedAt        time.Time `json:"cached_at"`
	Site            string    `json:"site"`
}

// saveCookiesToCache saves cookies to user cache directory
func saveCookiesToCache(session, csrfToken, cfClearance, site string) error {
	cacheDir := config.Get().CacheDir()
	credFile := filepath.Join(cacheDir, "credentials.json")

	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	creds := CachedCredentials{
		LeetCodeSession: session,
		CsrfToken:       csrfToken,
		CfClearance:     cfClearance,
		CachedAt:        time.Now(),
		Site:            site,
	}

	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	// Write to file with restricted permissions (0600 - owner read/write only)
	return os.WriteFile(credFile, data, 0600)
}

// loadCookiesFromCache loads cookies from cache
func loadCookiesFromCache(site string) (session, csrfToken, cfClearance string, err error) {
	credFile := filepath.Join(config.Get().CacheDir(), "credentials.json")

	data, err := os.ReadFile(credFile)
	if err != nil {
		return "", "", "", err
	}

	var creds CachedCredentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", "", "", err
	}

	// Verify site matches
	if creds.Site != site {
		return "", "", "", fmt.Errorf("cached credentials for different site")
	}

	return creds.LeetCodeSession, creds.CsrfToken, creds.CfClearance, nil
}

// clearCookiesFromCache removes cached cookies
func clearCookiesFromCache() {
	credFile := filepath.Join(config.Get().CacheDir(), "credentials.json")
	_ = os.Remove(credFile)
}
