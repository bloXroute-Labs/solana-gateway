package jwt

import (
	"fmt"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/golang-jwt/jwt/v5"
)

const (
	AccountIDClaim = "account_id"
)

func GenerateJWT(
	secret string,
	exp time.Time,
	claims map[string]interface{},
) (string, error) {
	now := time.Now().UTC()

	c := make(jwt.MapClaims)
	c["dat"] = "ofr-gateway"
	c["exp"] = exp.Unix() // expiration time
	c["iat"] = now.Unix() // issue time

	for k, v := range claims {
		c[k] = v
	}

	tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func ParseJWT(token string) (*jwt.Token, error) {
	var parser = jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
		jwt.WithoutClaimsValidation(),
	)

	parsedToken, _, err := parser.ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWT: %w", err)
	}

	return parsedToken, nil
}

func ValidateJWT(token string, secret string) (*jwt.Token, error) {
	var parser = jwt.NewParser(
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
	)

	tkn, err := parser.Parse(token, func(jwtToken *jwt.Token) (interface{}, error) {
		if _, ok := jwtToken.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected method: %s", jwtToken.Header["alg"])
		}

		return []byte(secret), nil
	})

	if err != nil {
		return nil, fmt.Errorf("JWT token validation: %w", err)
	}

	if !tkn.Valid {
		return nil, fmt.Errorf("JWT token is invalid")
	}

	claims, ok := tkn.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}

	expUnix, ok := claims["exp"]
	if !ok {
		return nil, fmt.Errorf("token exp not set")
	}

	expUnixFloat, ok := expUnix.(float64)
	if !ok {
		return nil, fmt.Errorf("token exp unix not int")
	}

	if expUnixFloat < float64(time.Now().UTC().Unix()) {
		return nil, fmt.Errorf("token expired")
	}

	return tkn, nil
}

func GetAccountIDFromJWT(token *jwt.Token) (types.AccountID, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims")
	}

	accountID, ok := claims[AccountIDClaim]
	if !ok {
		return "", fmt.Errorf("account_id not set")
	}

	accountIDStr, ok := accountID.(string)
	if !ok {
		return "", fmt.Errorf("account_id not string")
	}

	return types.AccountID(accountIDStr), nil
}

func GetExpirationTimeFromJWT(token *jwt.Token) (time.Time, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return time.Time{}, fmt.Errorf("invalid claims format")
	}

	expUnix, ok := claims["exp"]
	if !ok {
		return time.Time{}, fmt.Errorf("token exp not set")
	}

	expUnixFloat, ok := expUnix.(float64)
	if !ok {
		return time.Time{}, fmt.Errorf("token exp unix not int")
	}

	return time.Unix(int64(expUnixFloat), 0), nil
}
