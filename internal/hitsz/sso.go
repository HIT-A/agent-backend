package hitsz

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	CombinedLoginURL = "https://ids.hit.edu.cn/authserver/combinedLogin.do?type=IDSUnion&appId=ff2dfca3a2a2448e9026a8c6e38fa52b&success=http%3A%2F%2Fjw.hitsz.edu.cn%2FcasLogin"
	IDSCallbackURL   = "https://ids.hit.edu.cn/authserver/callback"
	SSOBaseURL       = "https://sso.hitsz.edu.cn:7002"
)

type SSOClient struct {
	client   *http.Client
	username string
	password string
}

func NewSSOClient(username, password string) *SSOClient {
	jar, _ := cookiejar.New(nil)
	return &SSOClient{
		client: &http.Client{
			Jar:           jar,
			Timeout:       30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error { return nil },
		},
		username: username,
		password: password,
	}
}

type PageResult struct {
	URL   string     `json:"final_url"`
	Title string     `json:"title"`
	Links []PageLink `json:"links"`
	HTML  string     `json:"html"`
}

type PageLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type FetchResult struct {
	LoginSuccess bool      `json:"login_success"`
	FetchedPage  FetchPage `json:"fetched_page"`
}

type FetchPage struct {
	FinalURL string      `json:"final_url"`
	Title    string      `json:"title"`
	Links    []FetchLink `json:"links"`
	HTML     string      `json:"html"`
}

type FetchLink struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type authForm struct {
	Action   string
	ClientID string
	Scope    string
	State    string
}

func FetchPublicInfo(username, password, targetURL string) (*FetchResult, error) {
	client := NewSSOClient(username, password)

	authForm, err := client.getIDSLoginForm()
	if err != nil {
		return nil, err
	}

	if err := client.submitSSOLogin(authForm); err != nil {
		return nil, err
	}

	if targetURL == "" {
		targetURL = "https://info.hitsz.edu.cn/list.jsp?wbtreeid=1053"
	}

	pageResult, err := client.fetchPage(targetURL)
	if err != nil {
		return nil, err
	}

	return &FetchResult{
		LoginSuccess: true,
		FetchedPage:  pageResultToFetchPage(pageResult),
	}, nil
}

func (c *SSOClient) getIDSLoginForm() (*authForm, error) {
	resp, err := c.client.Get(CombinedLoginURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(html)))
	if err != nil {
		return nil, err
	}

	form := doc.Find("#authZForm")
	action, _ := form.Attr("action")

	formData := &authForm{Action: action}
	form.Find("input[name]").Each(func(i int, s *goquery.Selection) {
		name, _ := s.Attr("name")
		value, _ := s.Attr("value")
		switch name {
		case "client_id":
			formData.ClientID = value
		case "scope":
			formData.Scope = value
		case "state":
			formData.State = value
		}
	})

	return formData, nil
}

func (c *SSOClient) submitSSOLogin(form *authForm) error {
	ssoURL := SSOBaseURL + form.Action
	data := url.Values{
		"action":        {"authorize"},
		"response_type": {"code"},
		"redirect_uri":  {IDSCallbackURL},
		"client_id":     {form.ClientID},
		"scope":         {form.Scope},
		"state":         {form.State},
		"username":      {c.username},
		"password":      {c.password},
	}

	resp, err := c.client.PostForm(ssoURL, data)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *SSOClient) fetchPage(targetURL string) (*PageResult, error) {
	resp, err := c.client.Get(targetURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := string(body)
	return &PageResult{
		URL:   finalURL,
		Title: extractTitle(html),
		Links: extractLinks(html),
		HTML:  html,
	}, nil
}

func extractTitle(html string) string {
	re := regexp.MustCompile(`<title[^>]*>([^<]*)</title>`)
	if matches := re.FindStringSubmatch(html); len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func extractLinks(html string) []PageLink {
	var links []PageLink
	re := regexp.MustCompile(`<a[^>]*href="([^"]*)"[^>]*>([^<]*)</a>`)
	for _, match := range re.FindAllStringSubmatch(html, -1) {
		if len(match) > 2 {
			links = append(links, PageLink{
				Title: strings.TrimSpace(match[2]),
				URL:   strings.TrimSpace(match[1]),
			})
		}
	}
	return links
}

func pageResultToFetchPage(page *PageResult) FetchPage {
	links := make([]FetchLink, len(page.Links))
	for i, l := range page.Links {
		links[i] = FetchLink{Title: l.Title, URL: l.URL}
	}
	return FetchPage{
		FinalURL: page.URL,
		Title:    page.Title,
		Links:    links,
		HTML:     page.HTML,
	}
}
