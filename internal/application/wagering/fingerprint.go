package wagering

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"backend-challenge/internal/domain"
	"backend-challenge/internal/domain/money"
	domainwagering "backend-challenge/internal/domain/wagering"
)

type BusinessInput struct {
	ProviderID, ExternalTransactionID   string
	PlayerID, WalletID, RoundID, GameID string
	Kind                                domainwagering.Kind
	Money                               money.Money
	ReferenceExternalTransactionID      string
}

func CanonicalBusinessJSON(input BusinessInput) ([]byte, error) {
	if !input.Money.Valid() {
		return nil, domain.NewError(domain.ErrInvalidValue, "business input contains uninitialized money")
	}
	fields := map[string]any{
		"externalTransactionId":          input.ExternalTransactionID,
		"gameId":                         input.GameID,
		"kind":                           string(input.Kind),
		"money":                          map[string]any{"amount": input.Money.Amount(), "currency": input.Money.Currency().String()},
		"playerId":                       input.PlayerID,
		"providerId":                     input.ProviderID,
		"referenceExternalTransactionId": input.ReferenceExternalTransactionID,
		"roundId":                        input.RoundID,
		"walletId":                       input.WalletID,
	}
	var b bytes.Buffer
	if err := writeCanonical(&b, fields); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func Fingerprint(input BusinessInput) (string, error) {
	data, err := CanonicalBusinessJSON(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func writeCanonical(out *bytes.Buffer, value any) error {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			key, _ := json.Marshal(k)
			out.Write(key)
			out.WriteByte(':')
			if err := writeCanonical(out, v[k]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	case string:
		data, _ := json.Marshal(v)
		out.Write(data)
	case nil:
		out.WriteString("null")
	case bool:
		out.WriteString(strconv.FormatBool(v))
	default:
		return fmt.Errorf("unsupported canonical value %T", value)
	}
	return nil
}
