package okx

import (
	"context"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes okx as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/okx-cli/okx"
//
// The init below registers it; the host routes okx:// URIs to the operations
// Register installs. The same Domain also builds the standalone okx binary
// (see cli.NewApp), so the binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the okx driver. It carries no state.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "okx",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "okx",
			Short:  "A command line for OKX crypto exchange market data.",
			Long: `A command line for OKX crypto exchange market data.

okx reads public OKX market data over HTTPS and prints clean records that
pipe into the rest of your tools. No API key or account required.`,
			Site: Host,
			Repo: "https://github.com/tamnd/okx-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{
		Name:     "ticker",
		Group:    "market",
		Single:   true,
		Summary:  "Get current price and 24h stats for a symbol",
		URIType:  "ticker",
		Resolver: true,
		Args:     []kit.Arg{{Name: "symbol", Help: "instrument id, e.g. BTC-USDT"}},
	}, getTicker)

	kit.Handle(app, kit.OpMeta{
		Name:    "tickers",
		Group:   "market",
		List:    true,
		Summary: "List tickers for all instruments of a type",
	}, getTickers)

	kit.Handle(app, kit.OpMeta{
		Name:    "candles",
		Group:   "market",
		List:    true,
		Summary: "Fetch OHLCV candlestick bars for a symbol",
		Args:    []kit.Arg{{Name: "symbol", Help: "instrument id, e.g. BTC-USDT"}},
	}, getCandles)

	kit.Handle(app, kit.OpMeta{
		Name:    "instruments",
		Group:   "market",
		List:    true,
		Summary: "List trading instruments",
	}, getInstruments)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type tickerIn struct {
	Symbol string  `kit:"arg" help:"instrument id, e.g. BTC-USDT"`
	Client *Client `kit:"inject"`
}

type tickersIn struct {
	Type   string  `kit:"flag" help:"instrument type (SPOT/SWAP/FUTURES/OPTION)" default:"SPOT"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

type candlesIn struct {
	Symbol string  `kit:"arg" help:"instrument id, e.g. BTC-USDT"`
	Bar    string  `kit:"flag" help:"bar size (1m/5m/15m/30m/1H/4H/1D/1W/1M)" default:"1D"`
	Count  int     `kit:"flag" help:"number of bars (max 300)" default:"30"`
	Client *Client `kit:"inject"`
}

type instrumentsIn struct {
	Type   string  `kit:"flag" help:"instrument type (SPOT/SWAP/FUTURES)" default:"SPOT"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func getTicker(ctx context.Context, in tickerIn, emit func(*Ticker) error) error {
	t, err := in.Client.GetTicker(ctx, in.Symbol)
	if err != nil {
		return mapErr(err)
	}
	return emit(t)
}

func getTickers(ctx context.Context, in tickersIn, emit func(*Ticker) error) error {
	instType := in.Type
	if instType == "" {
		instType = "SPOT"
	}
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	tickers, err := in.Client.GetTickers(ctx, instType, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, t := range tickers {
		if err := emit(t); err != nil {
			return err
		}
	}
	return nil
}

func getCandles(ctx context.Context, in candlesIn, emit func(*Candle) error) error {
	bar := in.Bar
	if bar == "" {
		bar = "1D"
	}
	count := in.Count
	if count == 0 {
		count = 30
	}
	candles, err := in.Client.GetCandles(ctx, in.Symbol, bar, count)
	if err != nil {
		return mapErr(err)
	}
	for _, c := range candles {
		if err := emit(c); err != nil {
			return err
		}
	}
	return nil
}

func getInstruments(ctx context.Context, in instrumentsIn, emit func(*Instrument) error) error {
	instType := in.Type
	if instType == "" {
		instType = "SPOT"
	}
	limit := in.Limit
	if limit == 0 {
		limit = 20
	}
	instruments, err := in.Client.GetInstruments(ctx, instType, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, inst := range instruments {
		if err := emit(inst); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: URI string functions, pure and network-free ---

// Classify turns a symbol into (type, id).
func (Domain) Classify(input string) (uriType, id string, err error) {
	if input == "" {
		return "", "", errs.Usage("empty okx reference")
	}
	return "ticker", input, nil
}

// Locate returns the live OKX page URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "ticker", "candle", "instrument":
		return "https://" + Host + "/trade-spot/" + id, nil
	default:
		return "", errs.Usage("okx has no resource type %q", uriType)
	}
}

// mapErr converts library errors to kit error kinds.
func mapErr(err error) error {
	return err
}
