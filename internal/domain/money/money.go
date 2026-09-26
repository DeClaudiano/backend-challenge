package money

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"

	"backend-challenge/internal/domain"
)

type Currency string

const BRL Currency = "BRL"

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// iso4217Codes contains the active ISO 4217 alphabetic codes accepted by the
// challenge. Keeping the allow-list in the domain prevents syntactically valid
// but nonexistent codes from entering financial state.
var iso4217Codes = func() map[string]struct{} {
	codes := strings.Fields(`AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BOV BRL BSD BTN BWP BYN BZD CAD CDF CHE CHF CHW CLF CLP CNY COP COU CRC CUC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HRK HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MXV MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SLL SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD USN UYI UYU UYW UZS VED VES VND VUV WST XAF XAG XAU XBA XBB XBC XBD XCD XDR XOF XPD XPF XPT XSU XTS XUA XXX YER ZAR ZMW ZWG`)
	result := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		result[code] = struct{}{}
	}
	return result
}()

func NewCurrency(value string) (Currency, error) {
	if !currencyPattern.MatchString(value) {
		return "", domain.NewError(domain.ErrInvalidCurrency, "currency must be an uppercase ISO 4217 code")
	}
	if _, ok := iso4217Codes[value]; !ok {
		return "", domain.NewError(domain.ErrInvalidCurrency, "currency must be a supported ISO 4217 code")
	}
	return Currency(value), nil
}
func (c Currency) String() string { return string(c) }
func (c Currency) Valid() bool {
	if !currencyPattern.MatchString(string(c)) {
		return false
	}
	_, ok := iso4217Codes[string(c)]
	return ok
}

type Money struct {
	minor    int64
	currency Currency
}

func New(minor int64, currency Currency) (Money, error) {
	if !currency.Valid() {
		return Money{}, domain.NewError(domain.ErrInvalidCurrency, "invalid currency")
	}
	return Money{minor: minor, currency: currency}, nil
}
func Zero(currency Currency) (Money, error) { return New(0, currency) }
func Parse(amount string, currency Currency) (Money, error) {
	if !currency.Valid() {
		return Money{}, domain.NewError(domain.ErrInvalidCurrency, "invalid currency")
	}
	if amount == "" || strings.HasPrefix(amount, "-") || strings.HasPrefix(amount, "+") || strings.ContainsAny(amount, "eE") || amount == "NaN" || amount == "Infinity" {
		return Money{}, domain.NewError(domain.ErrInvalidValue, "amount must be a non-negative decimal with exactly two fractional digits")
	}
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.[0-9]{2}$`).MatchString(amount) {
		return Money{}, domain.NewError(domain.ErrInvalidValue, "amount must have exactly two fractional digits")
	}
	parts := strings.SplitN(amount, ".", 2)
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > math.MaxInt64/100 {
		return Money{}, domain.NewError(domain.ErrOverflow, "amount exceeds int64 minor-unit range")
	}
	fraction, _ := strconv.ParseInt(parts[1], 10, 64)
	minor := whole*100 + fraction
	if minor < 0 || whole == math.MaxInt64/100 && fraction > math.MaxInt64%100 {
		return Money{}, domain.NewError(domain.ErrOverflow, "amount exceeds int64 minor-unit range")
	}
	return Money{minor: minor, currency: currency}, nil
}
func (m Money) Minor() int64       { return m.minor }
func (m Money) Currency() Currency { return m.currency }
func (m Money) Valid() bool        { return m.currency.Valid() }
func (m Money) IsZero() bool       { return m.minor == 0 }
func (m Money) IsPositive() bool   { return m.minor > 0 }
func (m Money) IsNegative() bool   { return m.minor < 0 }
func (m Money) Amount() string {
	if m.minor < 0 {
		if m.minor == math.MinInt64 {
			return "-92233720368547758.08"
		}
		return "-" + formatMinor(-m.minor)
	}
	return formatMinor(m.minor)
}
func formatMinor(minor int64) string {
	return strconv.FormatInt(minor/100, 10) + "." + leftPad2(strconv.FormatInt(minor%100, 10))
}
func leftPad2(s string) string {
	if len(s) == 1 {
		return "0" + s
	}
	return s
}
func (m Money) String() string { return m.Amount() + " " + m.currency.String() }
func (m Money) Add(other Money) (Money, error) {
	if err := m.compatible(other); err != nil {
		return Money{}, err
	}
	if other.minor > 0 && m.minor > math.MaxInt64-other.minor || other.minor < 0 && m.minor < math.MinInt64-other.minor {
		return Money{}, domain.NewError(domain.ErrOverflow, "money addition overflow")
	}
	return Money{minor: m.minor + other.minor, currency: m.currency}, nil
}
func (m Money) Sub(other Money) (Money, error) {
	if err := m.compatible(other); err != nil {
		return Money{}, err
	}
	neg, err := other.Neg()
	if err != nil {
		return Money{}, err
	}
	return m.Add(neg)
}
func (m Money) Neg() (Money, error) {
	if m.minor == math.MinInt64 {
		return Money{}, domain.NewError(domain.ErrOverflow, "money negation overflow")
	}
	return Money{minor: -m.minor, currency: m.currency}, nil
}
func (m Money) Compare(other Money) (int, error) {
	if err := m.compatible(other); err != nil {
		return 0, err
	}
	if m.minor < other.minor {
		return -1, nil
	}
	if m.minor > other.minor {
		return 1, nil
	}
	return 0, nil
}
func (m Money) Equal(other Money) bool { return m.currency == other.currency && m.minor == other.minor }
func (m Money) compatible(other Money) error {
	if !m.currency.Valid() || !other.currency.Valid() {
		return domain.NewError(domain.ErrInvalidCurrency, "invalid currency")
	}
	if m.currency != other.currency {
		return domain.NewError(domain.ErrCurrencyMismatch, "money currencies differ")
	}
	return nil
}
func (m Money) MarshalJSON() ([]byte, error) {
	if !m.Valid() {
		return nil, domain.NewError(domain.ErrInvalidValue, "cannot serialize uninitialized money")
	}
	return json.Marshal(struct {
		Amount   string   `json:"amount"`
		Currency Currency `json:"currency"`
	}{m.Amount(), m.currency})
}
func (m *Money) UnmarshalJSON(data []byte) error {
	var v struct {
		Amount   string   `json:"amount"`
		Currency Currency `json:"currency"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(data), &v); err != nil {
		return err
	}
	parsed, err := Parse(v.Amount, v.Currency)
	if err != nil {
		return err
	}
	*m = parsed
	return nil
}
