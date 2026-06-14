package okx

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in okx_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "okx" {
		t.Errorf("Scheme = %q, want okx", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "okx" {
		t.Errorf("Identity.Binary = %q, want okx", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct{ in, typ, id string }{
		{"BTC-USDT", "ticker", "BTC-USDT"},
		{"ETH-USDT", "ticker", "ETH-USDT"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestLocate(t *testing.T) {
	got, err := Domain{}.Locate("ticker", "BTC-USDT")
	want := "https://" + Host + "/trade-spot/BTC-USDT"
	if err != nil || got != want {
		t.Errorf("Locate = (%q, %v), want (%q, nil)", got, err, want)
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "BTC-USDT")
	if err == nil {
		t.Error("Locate with unknown type should return an error")
	}
}

// TestHostWiring mounts the driver in a kit Host and checks the round trip.
func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	ticker := &Ticker{Symbol: "BTC-USDT", Last: "63790.1", Bid: "63790", Ask: "63790.2"}
	u, err := h.Mint(ticker)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "okx://ticker/BTC-USDT"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("okx", "ETH-USDT")
	if err != nil || got.String() != "okx://ticker/ETH-USDT" {
		t.Errorf("ResolveOn = (%q, %v), want okx://ticker/ETH-USDT", got.String(), err)
	}
}
