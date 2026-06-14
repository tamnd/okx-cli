// Package okx is the library behind the okx command line:
// the HTTP client, request shaping, and the typed data models for the
// OKX crypto exchange public market data API.
//
// The Client paces requests, retries transient errors (429 and 5xx), and sets
// an honest User-Agent. All four operations (ticker, tickers, candles,
// instruments) read from OKX public endpoints that need no API key.
package okx

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultUserAgent identifies the client to OKX.
const DefaultUserAgent = "okx-cli/0.1 (tamnd87@gmail.com)"

// Host is the OKX hostname this client talks to.
const Host = "www.okx.com"

// BaseURL is the root every API request is built from.
const BaseURL = "https://www.okx.com/api/v5"

// Client talks to the OKX public API over HTTPS.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   3,
	}
}

// wireResp is the OKX API envelope for all responses.
type wireResp[T any] struct {
	Code string `json:"code"`
	Msg  string `json:"msg"`
	Data []T    `json:"data"`
}

// Ticker holds current price and 24h stats for one instrument.
type Ticker struct {
	Symbol  string `kit:"id" json:"symbol"`
	Last    string `json:"last"`
	Bid     string `json:"bid"`
	Ask     string `json:"ask"`
	Open24h string `json:"open_24h"`
	High24h string `json:"high_24h"`
	Low24h  string `json:"low_24h"`
	Vol24h  string `json:"vol_24h"`
}

// wireTicker is the raw OKX ticker shape from the API.
type wireTicker struct {
	InstID    string `json:"instId"`
	Last      string `json:"last"`
	AskPx     string `json:"askPx"`
	BidPx     string `json:"bidPx"`
	Open24h   string `json:"open24h"`
	High24h   string `json:"high24h"`
	Low24h    string `json:"low24h"`
	Vol24h    string `json:"vol24h"`
	VolCcy24h string `json:"volCcy24h"`
}

func toTicker(w wireTicker) *Ticker {
	return &Ticker{
		Symbol:  w.InstID,
		Last:    w.Last,
		Bid:     w.BidPx,
		Ask:     w.AskPx,
		Open24h: w.Open24h,
		High24h: w.High24h,
		Low24h:  w.Low24h,
		Vol24h:  w.Vol24h,
	}
}

// Candle is one OHLCV bar for an instrument.
type Candle struct {
	Timestamp string `kit:"id" json:"timestamp"`
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	Close     string `json:"close"`
	Volume    string `json:"volume"`
}

// Instrument is a trading pair listed on OKX.
type Instrument struct {
	ID       string `kit:"id" json:"id"`
	Base     string `json:"base"`
	Quote    string `json:"quote"`
	MinSize  string `json:"min_size"`
	TickSize string `json:"tick_size"`
	State    string `json:"state"`
}

// wireInstrument is the raw OKX instrument shape from the API.
type wireInstrument struct {
	InstID  string `json:"instId"`
	BaseCcy string `json:"baseCcy"`
	QuoteCcy string `json:"quoteCcy"`
	MinSz   string `json:"minSz"`
	TickSz  string `json:"tickSz"`
	State   string `json:"state"`
}

func toInstrument(w wireInstrument) *Instrument {
	return &Instrument{
		ID:       w.InstID,
		Base:     w.BaseCcy,
		Quote:    w.QuoteCcy,
		MinSize:  w.MinSz,
		TickSize: w.TickSz,
		State:    w.State,
	}
}

// GetTicker fetches the current price and 24h stats for one instrument (e.g. "BTC-USDT").
func (c *Client) GetTicker(ctx context.Context, symbol string) (*Ticker, error) {
	url := BaseURL + "/market/ticker?instId=" + symbol
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp wireResp[wireTicker]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("ticker decode: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no ticker data for %s", symbol)
	}
	return toTicker(resp.Data[0]), nil
}

// GetTickers fetches all tickers for an instrument type (SPOT, SWAP, FUTURES, OPTION).
// Results are capped at limit; pass 0 for no cap.
func (c *Client) GetTickers(ctx context.Context, instType string, limit int) ([]*Ticker, error) {
	url := BaseURL + "/market/tickers?instType=" + instType
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp wireResp[wireTicker]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("tickers decode: %w", err)
	}
	out := make([]*Ticker, 0, len(resp.Data))
	for _, w := range resp.Data {
		out = append(out, toTicker(w))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// GetCandles fetches OHLCV candlestick bars for a symbol.
// bar is one of 1m, 5m, 15m, 30m, 1H, 4H, 1D, 1W, 1M.
// count is the number of bars; OKX max is 300.
func (c *Client) GetCandles(ctx context.Context, symbol, bar string, count int) ([]*Candle, error) {
	url := fmt.Sprintf("%s/market/candles?instId=%s&bar=%s&limit=%d", BaseURL, symbol, bar, count)
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	// candles: data is [][]json.RawMessage (array of arrays)
	var resp struct {
		Code string              `json:"code"`
		Msg  string              `json:"msg"`
		Data [][]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("candles decode: %w", err)
	}
	out := make([]*Candle, 0, len(resp.Data))
	for _, row := range resp.Data {
		if len(row) < 6 {
			continue
		}
		var ts, open, high, low, close_, vol string
		// each element is a quoted JSON string
		_ = json.Unmarshal(row[0], &ts)
		_ = json.Unmarshal(row[1], &open)
		_ = json.Unmarshal(row[2], &high)
		_ = json.Unmarshal(row[3], &low)
		_ = json.Unmarshal(row[4], &close_)
		_ = json.Unmarshal(row[5], &vol)
		out = append(out, &Candle{
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    vol,
		})
	}
	return out, nil
}

// GetInstruments fetches the list of trading instruments for an instrument type.
// Results are capped at limit; pass 0 for no cap.
func (c *Client) GetInstruments(ctx context.Context, instType string, limit int) ([]*Instrument, error) {
	url := BaseURL + "/public/instruments?instType=" + instType
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	var resp wireResp[wireInstrument]
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("instruments decode: %w", err)
	}
	out := make([]*Instrument, 0, len(resp.Data))
	for _, w := range resp.Data {
		out = append(out, toInstrument(w))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Get fetches rawURL and returns the body. It paces and retries transient errors.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
