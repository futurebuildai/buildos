package currency

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	cases := []struct {
		code string
		ok   bool
	}{
		{USD, true},
		{CAD, true},
		{"usd", false},
		{"EUR", false},
		{"", false},
		{"USDX", false},
	}
	for _, c := range cases {
		err := Validate(c.code)
		if (err == nil) != c.ok {
			t.Errorf("Validate(%q): err=%v, want ok=%v", c.code, err, c.ok)
		}
		if err != nil && !errors.Is(err, ErrUnsupportedCurrency) {
			t.Errorf("Validate(%q): err=%v should wrap ErrUnsupportedCurrency", c.code, err)
		}
	}
}

func TestNew(t *testing.T) {
	m, err := New(12345, USD)
	if err != nil {
		t.Fatalf("New(12345, USD): %v", err)
	}
	if m.Cents != 12345 || m.CurrencyCode != USD {
		t.Errorf("New(12345, USD) = %+v", m)
	}

	if _, err := New(0, "EUR"); err == nil {
		t.Error("New(0, EUR) should fail")
	}
	if _, err := New(0, ""); err == nil {
		t.Error("New(0, \"\") should fail; empty must be explicit")
	}
}

func TestZero(t *testing.T) {
	m, err := Zero(USD)
	if err != nil {
		t.Fatalf("Zero(USD): %v", err)
	}
	if m.Cents != 0 || m.CurrencyCode != USD {
		t.Errorf("Zero(USD) = %+v, want {0 USD}", m)
	}

	// An unsupported code propagates New's validation error.
	if _, err := Zero("EUR"); err == nil {
		t.Error("Zero(EUR) should fail (unsupported currency)")
	}
}

func TestAdd(t *testing.T) {
	a, _ := New(100, USD)
	b, _ := New(50, USD)

	got, err := a.Add(b)
	if err != nil || got.Cents != 150 || got.CurrencyCode != USD {
		t.Errorf("Add same-currency: got=%+v err=%v", got, err)
	}

	c, _ := New(50, CAD)
	if _, err := a.Add(c); !errors.Is(err, ErrCrossCurrency) {
		t.Errorf("Add USD+CAD: err=%v, want ErrCrossCurrency", err)
	}
}

func TestSub(t *testing.T) {
	a, _ := New(100, USD)
	b, _ := New(40, USD)

	got, err := a.Sub(b)
	if err != nil || got.Cents != 60 || got.CurrencyCode != USD {
		t.Errorf("Sub same-currency: got=%+v err=%v", got, err)
	}

	// Negative is allowed — represents a debt or refund.
	got, err = b.Sub(a)
	if err != nil || got.Cents != -60 {
		t.Errorf("Sub negative result: got=%+v err=%v", got, err)
	}

	c, _ := New(40, CAD)
	if _, err := a.Sub(c); !errors.Is(err, ErrCrossCurrency) {
		t.Errorf("Sub USD-CAD: err=%v, want ErrCrossCurrency", err)
	}
}

func TestSameCurrency(t *testing.T) {
	usdA := Money{Cents: 1, CurrencyCode: USD}
	usdB := Money{Cents: 2, CurrencyCode: USD}
	cad := Money{Cents: 1, CurrencyCode: CAD}
	zero := Money{}

	if !usdA.SameCurrency(usdB) {
		t.Error("USD vs USD should be same")
	}
	if usdA.SameCurrency(cad) {
		t.Error("USD vs CAD should differ")
	}
	if usdA.SameCurrency(zero) {
		t.Error("USD vs zero-value should differ — never aggregate uninitialized Money")
	}
	if zero.SameCurrency(zero) {
		t.Error("zero vs zero should differ — protects against silent aggregation")
	}
}

func TestSumByCurrency(t *testing.T) {
	values := []Money{
		{Cents: 100, CurrencyCode: USD},
		{Cents: 50, CurrencyCode: CAD},
		{Cents: 200, CurrencyCode: USD},
		{Cents: 25, CurrencyCode: CAD},
		{Cents: 999, CurrencyCode: ""}, // ignored
	}
	got := SumByCurrency(values)
	if got[USD] != 300 {
		t.Errorf("USD total = %d, want 300", got[USD])
	}
	if got[CAD] != 75 {
		t.Errorf("CAD total = %d, want 75", got[CAD])
	}
	if _, ok := got[""]; ok {
		t.Error("empty currency code should not appear as a key")
	}
}

func TestErrCrossCurrencyMessageIncludesCodes(t *testing.T) {
	a, _ := New(1, USD)
	b, _ := New(1, CAD)
	_, err := a.Add(b)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, USD) || !strings.Contains(msg, CAD) {
		t.Errorf("error message should name both currencies; got: %s", msg)
	}
}
