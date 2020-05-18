package bugzilla

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// bugzillaCGIClient bugzilla REST API client
type bugzillaCGIClient struct {
	bugzillaAddr                    string
	httpClient                      *http.Client
	bugzillaLogin, bugzillaPassword string
}

const httpTimeout int = 60

// newHTTPClient creates HTTP client for HTTP based endpoints
func newHTTPClient() (*http.Client, error) {
	timeout := time.Duration(httpTimeout) * time.Second
	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	client := http.Client{
		Jar:     cookieJar,
		Timeout: timeout,
	}
	return &client, nil
}

// NewCGIClient creates a helper json rpc client for regular HTTP based endpoints
func newCGIClient(endpoint, bugzillaLogin, bugzillaPassword string) (*bugzillaCGIClient, error) {
	client, err := newHTTPClient()
	if err != nil {
		return nil, err
	}
	return &bugzillaCGIClient{
		bugzillaAddr:     endpoint,
		httpClient:       client,
		bugzillaLogin:    bugzillaLogin,
		bugzillaPassword: bugzillaPassword,
	}, nil
}

// setBugzillaLoginCookie visits bugzilla page to obtain login cookie
func (c *bugzillaCGIClient) setBugzillaLoginCookie(loginURL string) (err error) {
	req, err := newHTTPRequest("GET", loginURL, nil)
	if err != nil {
		return err
	}

	res, err := c.httpClient.Do(req)
	defer func() {
		closeBody(res)
	}()

	if err != nil {
		if strings.Contains(err.Error(), "use of closed network connection") {
			return fmt.Errorf("timeout occured while accessing %v", loginURL)
		}
		return err
	}
	return nil
}

// getBugzillaLoginToken returns Bugzilla_login_token input field value. Requires login cookie to be set
func (c *bugzillaCGIClient) getBugzillaLoginToken(loginURL string) (loginToken string, err error) {
	req, err := newHTTPRequest("GET", loginURL, nil)
	if err != nil {
		return "", err
	}

	res, err := c.httpClient.Do(req)
	defer func() {
		defer closeBody(res)
	}()
	if err != nil {
		if strings.Contains(err.Error(), "use of closed network connection") {
			return "", fmt.Errorf("timeout occured while accessing %v", loginURL)
		}
		return "", err
	}
	r := regexp.MustCompile(`name="Bugzilla_login_token"\s+value="(?P<value>[\d\w-]+)"`)
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	match := r.FindStringSubmatch(string(body))
	a := make(map[string]string)
	for i, name := range r.SubexpNames() {
		a[name] = match[i]
	}
	return a["value"], nil
}

// Login allows to login using Bugzilla CGI API
func (c *bugzillaCGIClient) login() (err error) {
	log.Printf("Authenticating to %q ...", c.bugzillaAddr)

	u, err := url.Parse(c.bugzillaAddr)
	if err != nil {
		return err
	}
	u.Path = "index.cgi"
	loginURL := u.String()

	err = c.setBugzillaLoginCookie(loginURL)
	if err != nil {
		return err
	}

	loginToken, err := c.getBugzillaLoginToken(loginURL)
	if err != nil {
		return err
	}

	data := url.Values{}
	data.Set("Bugzilla_login", c.bugzillaLogin)
	data.Set("Bugzilla_password", c.bugzillaPassword)
	data.Set("Bugzilla_login_token", loginToken)

	req, err := newHTTPRequest("POST", loginURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := c.httpClient.Do(req)
	defer closeBody(res)
	if err != nil {
		if strings.Contains(err.Error(), "use of closed network connection") {
			return fmt.Errorf("timeout occured while accessing %v", loginURL)
		}
		return err
	}

	return nil
}

func (c *bugzillaCGIClient) GetCookies() []*http.Cookie {
	u, err := url.Parse(c.bugzillaAddr)
	if err != nil {
		panic(err)
	}
	cookies := c.httpClient.Jar.Cookies(u)
	return cookies
}

func (c *bugzillaCGIClient) SetCookies(cookies []*http.Cookie) {
	u, _ := url.Parse(c.bugzillaAddr)
	c.httpClient.Jar.SetCookies(u, cookies)
}

func closeBody(r *http.Response) {
	if r != nil && r.Body != nil {
		if err := r.Body.Close(); err != nil {
			log.Printf("Failed to close body: %v", err)
		}
	}
}

func (c *bugzillaCGIClient) authenticated(f func() (*http.Response, error)) (*http.Response, error) {
	res, err := f()
	if err != nil {
		return nil, err
	}
	defer closeBody(res)
	bs, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	res.Body = ioutil.NopCloser(bytes.NewBuffer(bs))

	if strings.Contains(string(bs), "needs a legitimate login") || strings.Contains(string(bs), "Parameters Required") {
		if err := c.login(); err != nil {
			return nil, err
		}
		res, err = f()
		if err != nil {
			return nil, err
		}
	}

	return res, nil
}
