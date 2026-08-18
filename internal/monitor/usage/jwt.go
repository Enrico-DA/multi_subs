package usage

import (
	"encoding/base64"
	"encoding/json"
	"strings"
)

func decodeJWTPayloadSegment(token string) []byte {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 || parts[1] == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}
	return raw
}

func oauthAccountIDFromJWT(token string) string {
	payload := decodeJWTPayloadSegment(token)
	if len(payload) == 0 {
		return ""
	}
	var claims map[string]any
	if json.Unmarshal(payload, &claims) != nil {
		return ""
	}
	if id := oauthAccountIDHeaderValue(anyStringClaim(claims["chatgpt_account_id"])); id != "" {
		return id
	}
	if id := oauthAccountIDHeaderValue(anyStringClaim(claims["account_id"])); id != "" {
		return id
	}
	nested, ok := claims["https://api.openai.com/auth"].(map[string]any)
	if !ok {
		return ""
	}
	if id := oauthAccountIDHeaderValue(anyStringClaim(nested["chatgpt_account_id"])); id != "" {
		return id
	}
	return oauthAccountIDHeaderValue(anyStringClaim(nested["account_id"]))
}

func anyStringClaim(value any) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func resolveOAuthAccountID(payload authFilePayload) string {
	if id := oauthAccountIDHeaderValue(payload.Tokens.AccountID); id != "" {
		return id
	}
	if id := oauthAccountIDHeaderValue(payload.AccountID); id != "" {
		return id
	}
	if id := oauthAccountIDFromJWT(payload.Tokens.IDToken); id != "" {
		return id
	}
	return oauthAccountIDFromJWT(payload.Tokens.AccessToken)
}
