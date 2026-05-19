package oauth2

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
)

// pkceVerifierAlphabet is the RFC 7636 §4.1 unreserved character set:
//
//	A-Z / a-z / 0-9 / "-" / "." / "_" / "~"
//
// 66 characters total. Picking from it uniformly yields ~6.04 bits per
// char; a 43-char verifier therefore carries ~256 bits of entropy, well
// past the 128-bit floor RFC 7636 sets.
const pkceVerifierAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

// pkceVerifierLength is the length we generate. 43 is also the minimum
// length permitted by RFC 7636 §4.1, so any conforming verifier we emit is
// at least as long as anything the spec allows.
const pkceVerifierLength = 43

// generatePKCEVerifier returns a fresh RFC 7636 §4.1 code_verifier — a
// crypto/rand-sourced string of pkceVerifierLength characters drawn from
// the unreserved alphabet.
//
// The implementation rejects bytes that fall in the rolled-over tail of
// 256 % 66 so the distribution is exactly uniform. The loop's expected
// iteration count is well under 2× the output length even in the worst
// case.
func generatePKCEVerifier() (string, error) {
	const alpha = pkceVerifierAlphabet
	out := make([]byte, 0, pkceVerifierLength)
	// Pull bytes in small batches to amortize the rand.Read call without
	// over-reading when most bytes are usable.
	buf := make([]byte, pkceVerifierLength)
	// Largest multiple of len(alpha) that fits in a byte; bytes >= cutoff are discarded
	// to keep the modulo distribution uniform.
	cutoff := byte(256 - (256 % len(alpha)))
	for len(out) < pkceVerifierLength {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("failed to read random bytes for pkce verifier: %w", err)
		}
		for _, b := range buf {
			if b >= cutoff {
				continue
			}
			out = append(out, alpha[int(b)%len(alpha)])
			if len(out) == pkceVerifierLength {
				break
			}
		}
	}
	return string(out), nil
}

// pkceChallengeFor produces the code_challenge value that pairs with the
// given verifier under the named method.
//
//   - S256: base64url(sha256(verifier)) without padding (RFC 7636 §4.2).
//   - plain: the verifier verbatim — included for connectors that cannot
//     validate S256. Plain offers no defense against an attacker who can
//     read the authorize redirect, which is the threat PKCE exists to
//     mitigate; prefer S256 unless a provider forces our hand.
//
// Returns an error for an unknown method so a future enum extension can't
// silently fall through and emit no challenge at all.
func pkceChallengeFor(method sconfig.PKCEMethod, verifier string) (string, error) {
	switch method {
	case sconfig.PKCEMethodS256:
		sum := sha256.Sum256([]byte(verifier))
		return base64.RawURLEncoding.EncodeToString(sum[:]), nil
	case sconfig.PKCEMethodPlain:
		return verifier, nil
	default:
		return "", fmt.Errorf("unsupported pkce method %q", method)
	}
}
