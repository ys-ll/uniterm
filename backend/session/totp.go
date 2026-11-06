package session

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/ys-ll/uniterm/backend/credentials"
	"golang.org/x/crypto/ssh"
)

var otpPrompt = regexp.MustCompile(`(?i)(\b(?:totp|otp)\b|\bverification[ -]+code\b|\bone[ -]+time[ -]+(?:password|code)\b|\bauthentication[ -]+code\b|\bauthenticator[ -]+code\b)`)
var passwordPrompt = regexp.MustCompile(`(?i)\bpassword[0-9]*\b`)

func isPasswordPrompt(question string) bool {
	return passwordPrompt.MatchString(question) && !otpPrompt.MatchString(question)
}

func decodeTOTPSecret(secret string) ([]byte, error) {
	secret = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, secret)
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.TrimRight(secret, "="))
	if err != nil || len(key) < 10 {
		return nil, fmt.Errorf("invalid TOTP secret: enter a Base32 secret of at least 16 characters")
	}
	return key, nil
}

// ValidateTOTPConfig never includes the secret in errors. Existing ciphertext is
// preserved while the vault is locked; it must be unlocked before generating a code.
func ValidateTOTPConfig(config ConnectionConfig) error {
	if !config.TOTPEnabled || credentials.IsEncrypted(config.TOTPSecret) {
		return nil
	}
	_, err := decodeTOTPSecret(config.TOTPSecret)
	return err
}

// totpCode implements RFC 6238 with SHA-1, six digits and a 30-second period.
func totpCode(secret string, now time.Time) (string, error) {
	if credentials.IsEncrypted(secret) {
		return "", fmt.Errorf("unlock the credential vault to use TOTP")
	}
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return "", err
	}
	if now.Unix() < 0 {
		return "", fmt.Errorf("invalid system time for TOTP")
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(now.Unix()/30))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	value := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1000000), nil
}

// automaticTOTPChallenge answers each recognized field once per handshake.
// Unknown or rejected questions go to the terminal callback; unattended callers
// fail promptly. Secrets and generated codes are never printed to the terminal.
func automaticTOTPChallenge(config ConnectionConfig, fallback ssh.KeyboardInteractiveChallenge) ssh.KeyboardInteractiveChallenge {
	usedOTP, usedPassword := false, false
	return func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		if len(questions) != len(echos) {
			return nil, fmt.Errorf("invalid SSH authentication challenge")
		}
		answers := make([]string, len(questions))
		var pending []string
		var pendingEcho []bool
		var indices []int
		for i, question := range questions {
			switch {
			case !echos[i] && otpPrompt.MatchString(question) && !usedOTP:
				code, err := totpCode(config.TOTPSecret, time.Now())
				if err != nil {
					return nil, err
				}
				answers[i], usedOTP = code, true
			case !echos[i] && isPasswordPrompt(question) && !usedPassword &&
				(config.AuthType == "password" || config.AuthType == "") && config.Password != "" && !credentials.IsEncrypted(config.Password):
				answers[i], usedPassword = config.Password, true
			default:
				pending = append(pending, question)
				pendingEcho = append(pendingEcho, echos[i])
				indices = append(indices, i)
			}
		}
		if len(pending) > 0 {
			if fallback == nil {
				return nil, fmt.Errorf("SSH authentication requires manual input")
			}
			manual, err := fallback(user, instruction, pending, pendingEcho)
			if err != nil {
				return nil, err
			}
			if len(manual) != len(indices) {
				return nil, fmt.Errorf("invalid SSH authentication answers")
			}
			for j, i := range indices {
				answers[i] = manual[j]
			}
		}
		return answers, nil
	}
}
