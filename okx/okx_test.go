package okx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	c := NewClient()
	c.Rate = 0
	c.HTTP = &http.Client{Timeout: 5 * time.Second}
	return c
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestGetTicker(t *testing.T) {
	payload := wireResp[wireTicker]{
		Code: "0",
		Data: []wireTicker{
			{
				InstID:  "BTC-USDT",
				Last:    "63790.1",
				AskPx:   "63790.2",
				BidPx:   "63790",
				Open24h: "64024.6",
				High24h: "64214.8",
				Low24h:  "63683.9",
				Vol24h:  "5289.32",
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		if r.URL.Query().Get("instId") != "BTC-USDT" {
			t.Errorf("instId = %q, want BTC-USDT", r.URL.Query().Get("instId"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Override BaseURL temporarily via a direct Get
	body, err := c.Get(context.Background(), srv.URL+"?instId=BTC-USDT")
	if err != nil {
		t.Fatal(err)
	}

	var resp wireResp[wireTicker]
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) == 0 {
		t.Fatal("no data in response")
	}
	got := toTicker(resp.Data[0])
	if got.Symbol != "BTC-USDT" {
		t.Errorf("Symbol = %q, want BTC-USDT", got.Symbol)
	}
	if got.Last != "63790.1" {
		t.Errorf("Last = %q, want 63790.1", got.Last)
	}
	if got.Ask != "63790.2" {
		t.Errorf("Ask = %q, want 63790.2", got.Ask)
	}
	if got.Bid != "63790" {
		t.Errorf("Bid = %q, want 63790", got.Bid)
	}
}

func TestGetTickers(t *testing.T) {
	tickers := []wireTicker{
		{InstID: "BTC-USDT", Last: "63790.1", Vol24h: "5289.32"},
		{InstID: "ETH-USDT", Last: "3450.5", Vol24h: "12345.67"},
		{InstID: "SOL-USDT", Last: "142.3", Vol24h: "9876.54"},
	}
	payload := wireResp[wireTicker]{Code: "0", Data: tickers}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	var resp wireResp[wireTicker]
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}

	// apply limit client-side
	limit := 2
	var out []*Ticker
	for _, w := range resp.Data {
		out = append(out, toTicker(w))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	if len(out) != 2 {
		t.Errorf("got %d tickers, want 2", len(out))
	}
	if out[0].Symbol != "BTC-USDT" {
		t.Errorf("first symbol = %q, want BTC-USDT", out[0].Symbol)
	}
}

func TestGetCandles(t *testing.T) {
	// OKX candles: data is array of arrays
	payload := map[string]interface{}{
		"code": "0",
		"data": [][]string{
			{"1781452800000", "64024.6", "64214.8", "63683.9", "63790.1", "542.871", "34732047.7", "34732047.7", "1"},
			{"1781366400000", "63500.0", "64100.0", "63200.0", "64024.6", "610.23", "38900000.0", "38900000.0", "1"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	var resp struct {
		Code string              `json:"code"`
		Data [][]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("got %d rows, want 2", len(resp.Data))
	}

	var ts, open, close_ string
	_ = json.Unmarshal(resp.Data[0][0], &ts)
	_ = json.Unmarshal(resp.Data[0][1], &open)
	_ = json.Unmarshal(resp.Data[0][4], &close_)

	if ts != "1781452800000" {
		t.Errorf("ts = %q, want 1781452800000", ts)
	}
	if open != "64024.6" {
		t.Errorf("open = %q, want 64024.6", open)
	}
	if close_ != "63790.1" {
		t.Errorf("close = %q, want 63790.1", close_)
	}
}

func TestGetInstruments(t *testing.T) {
	instruments := []wireInstrument{
		{InstID: "BTC-USDT", BaseCcy: "BTC", QuoteCcy: "USDT", MinSz: "0.00001", TickSz: "0.1", State: "live"},
		{InstID: "ETH-USDT", BaseCcy: "ETH", QuoteCcy: "USDT", MinSz: "0.0001", TickSz: "0.01", State: "live"},
	}
	payload := wireResp[wireInstrument]{Code: "0", Data: instruments}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}

	var resp wireResp[wireInstrument]
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("got %d instruments, want 2", len(resp.Data))
	}

	got := toInstrument(resp.Data[0])
	if got.ID != "BTC-USDT" {
		t.Errorf("ID = %q, want BTC-USDT", got.ID)
	}
	if got.Base != "BTC" {
		t.Errorf("Base = %q, want BTC", got.Base)
	}
	if got.Quote != "USDT" {
		t.Errorf("Quote = %q, want USDT", got.Quote)
	}
	if got.State != "live" {
		t.Errorf("State = %q, want live", got.State)
	}
}

func TestGetUserAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if ua == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := NewClient()
	c.Rate = 0
	_, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
}
