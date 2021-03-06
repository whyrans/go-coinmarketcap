package coinmarketcap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL     = "https://pro-api.coinmarketcap.com/"
	defaultSearchURL   = "https://pro.coinmarketcap.com/"
	defalutVerAPI      = "v1/"
	searchInforAPIPath = "api/"

	sanboxBaseURL   = "https://sandbox-api.coinmarketcap.com/"
	sanboxSearchURL = "https://sandbox.coinmarketcap.com/"

	ResponseSuccessful     = 200
	ResponseBadRequest     = 400
	ReponseUnauthorized    = 401
	ResponseForbidden      = 403
	ReponseTooManyRequest  = 429
	ResponseInternalServer = 500

	MinLimitOption = 1
	MaxLimitOption = 5000
)

type Client struct {
	client    *http.Client // Http Client use to communicate with the API.
	BaseURL   *url.URL     // BaseURL for API request.
	SearchURL *url.URL     // SearchURL for search information request.

	// Services used for talking to different parts of the Coinmarketcap API.
	Cryptocurrency *CryptocurrencyService
	// Exchange       *ExchangeService
	// GlobalMetrics  *GlobalMetricsService
	// Tools          *ToolsService
	Search *SearchService

	RateLimit *RateLimit // Rate limits for the client as determined by the most recent API calls.
	config    *ConfigService
}

type ConfigService struct {
	AutoCheckRateLimit bool
}

type service struct {
	client *Client
}

type ListOptions struct {
	// Optionally offset the start (1-based index)
	// of the paginated list of items to return
	Start int `json:"start,omitempty" bson:"start,omitempty"`

	// Optionally specify the number of results to return.
	// Use this parameter and the "start" parameter to determine your own pagination size.
	Limit int `json:"limit,omitempty" bson:"limit,omitempty"`

	// Optionally calculate market quotes in up to 32 currencies
	// at once by passing a comma-separated list of cryptocurrency
	// or fiat currency symbols.
	// Each additional convert option beyond the first requires an additional call credit.
	Convert string `json:"convert,omitempty" bson:"convert,omitempty"`

	// Default: "market_cap".
	// What field to sort the list of cryptocurrencies by.
	Sort string `json:"sort,omitempty" bson:"sort,omitempty"`

	// The direction in which to order cryptocurrencies against the specified sort.
	SortDir string `json:"sort_dir,omitempty" bson:"sort_dir,omitempty"`

	// Default: "all".
	// The type of cryptocurrency to include.
	CryptocurrencyType string `json:"cryptocurrency_type,omitempty" bson:"cryptocurrency_type,omitempty"`

	ApiKey string `json:"api_key" bson:"api_key"`
}

func NewDefaultClient() *Client {
	// For testing
	urlAPI := sanboxBaseURL + defalutVerAPI
	urlSearch := sanboxSearchURL + searchInforAPIPath

	// urlAPI := defaultBaseURL + defalutVerAPI
	// urlSearch := defaultSearchURL + searchInforAPIPath

	baseURL, _ := url.Parse(urlAPI)
	searchURL, _ := url.Parse(urlSearch)
	c := &Client{
		client:    http.DefaultClient,
		BaseURL:   baseURL,
		SearchURL: searchURL,
		config: &ConfigService{
			AutoCheckRateLimit: false,
		},
	}

	c.Cryptocurrency = &CryptocurrencyService{client: c}
	c.Search = &SearchService{client: c}
	return c
}

func NewClient(httpClient *http.Client, versionAPI string) *Client {
	if httpClient == nil && versionAPI == "" {
		return NewDefaultClient()
	}

	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	if versionAPI == "" {
		versionAPI = defalutVerAPI
	}

	URL := defaultBaseURL + versionAPI
	baseURL, _ := url.Parse(URL)

	return &Client{
		client:  httpClient,
		BaseURL: baseURL,
	}
}

func (c *Client) NewRequest(method, urlString string, body interface{}) (*http.Request, error) {
	if !strings.HasSuffix(c.BaseURL.Path, "/") {
		return nil, fmt.Errorf("Base URL must have a trailing slash, but %s does not.", c.BaseURL)
	}

	u, errParse := c.BaseURL.Parse(urlString)
	if errParse != nil {
		return nil, errParse
	}

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		err := enc.Encode(body)
		if err != nil {
			return nil, err
		}
	}

	log.Println(u.String())
	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) NewSearchRequest(method, urlString string, body interface{}) (*http.Request, error) {
	if !strings.HasSuffix(c.SearchURL.Path, "/") {
		return nil, fmt.Errorf("Base URL must have a trailing slash, but %s does not.", c.SearchURL)
	}

	u, errParse := c.SearchURL.Parse(urlString)
	if errParse != nil {
		return nil, errParse
	}

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		err := enc.Encode(body)
		if err != nil {
			return nil, err
		}
	}

	log.Println(u.String())
	req, err := http.NewRequest(method, u.String(), buf)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func (c *Client) Do(ctx context.Context, req *http.Request, v interface{}) (*Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}

	response := newResponse(resp)
	defer resp.Body.Close()

	body, errReadBody := ioutil.ReadAll(resp.Body)
	if errReadBody != nil {
		return nil, err
	}

	if resp.StatusCode != ResponseSuccessful {
		return nil, fmt.Errorf("%s", body)
	}

	if err := json.Unmarshal(body, v); err != nil {
		return nil, err
	}

	return response, nil
}

type Response struct {
	*http.Response

	Rate
}

type Rate struct {
	// The number of requests per hour the client is currenly limited to.
	Limit int `json:"limit"`

	// The number of remaining requests the client can make this hour.
	Remaining int `json:"remaining"`

	// The time at which the current rate limit will reset.
	Reset time.Time `json:"reset"`
}

type RateLimit struct {
	// The rate limit for non-search API requests.
	Core *Rate

	// The rate limit for search API requests.
	Search *Rate
}

func (rl *RateLimit) SetCoreRate(limit int, remaining int, reset time.Time) {
	rl.Core.Limit = limit
	rl.Core.Remaining = remaining
	rl.Core.Reset = reset
}

func (rl *RateLimit) SetSearchRate(limit int, remaining int, reset time.Time) {
	rl.Search.Limit = limit
	rl.Search.Remaining = remaining
	rl.Search.Reset = reset
}

// Todo: Get rateLimit for requests
func newResponse(resHttp *http.Response) *Response {
	return &Response{Response: resHttp}
}

type ExchangeService service
type GlobalMetricsService service
type ToolsService service
