package localworkspace

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"
)

const crockfordBase32 = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func NewID(prefix string, now time.Time) (string, error) {
	if err := validateIDPrefix(prefix); err != nil {
		return "", err
	}
	buf := make([]byte, 16)
	millis := uint64(now.UTC().UnixMilli())
	buf[0] = byte(millis >> 40)
	buf[1] = byte(millis >> 32)
	buf[2] = byte(millis >> 24)
	buf[3] = byte(millis >> 16)
	buf[4] = byte(millis >> 8)
	buf[5] = byte(millis)
	if _, err := rand.Read(buf[6:]); err != nil {
		return "", WrapError(ErrorInternal, "generate id randomness", 5, err)
	}
	return fmt.Sprintf("%s_%s", prefix, encodeCrockford128(buf)), nil
}

func validateIDPrefix(prefix string) error {
	if strings.TrimSpace(prefix) == "" {
		return NewError(ErrorInvalidArgument, "id prefix is empty", 1, nil)
	}
	for _, ch := range prefix {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return NewError(ErrorInvalidArgument, "id prefix is not path safe", 1, map[string]string{"prefix": prefix})
	}
	return nil
}

func encodeCrockford128(data []byte) string {
	var n big.Int
	n.SetBytes(data)
	base := big.NewInt(32)
	zero := big.NewInt(0)
	out := make([]byte, 26)
	for i := len(out) - 1; i >= 0; i-- {
		if n.Cmp(zero) == 0 {
			out[i] = crockfordBase32[0]
			continue
		}
		var rem big.Int
		n.DivMod(&n, base, &rem)
		out[i] = crockfordBase32[rem.Int64()]
	}
	return string(out)
}
