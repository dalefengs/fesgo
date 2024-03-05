package token

import (
	"github.com/dalefeng/fesgo"
	"github.com/golang-jwt/jwt/v4"
	"time"
)

type JwtHandler struct {
	Authenticator func(ctx *fesgo.Context) (map[string]any, error)
	Alg           string        // 签名算法
	Secret        string        // 签名密钥
	PrivateKeys   []string      // 私钥
	Expire        time.Duration // 过期时间
	expireFunc    func() time.Time
	RefreshExpire time.Duration // 刷新token过期时间
	SendCookie    bool          // 是否发送cookie
	CookieName    string        // cookie名称
}

type JwtResponse struct {
	Token        string
	RefreshToken string
}

func (j *JwtHandler) LoginHandler(ctx *fesgo.Context) (*JwtResponse, error) {
	data, err := j.Authenticator(ctx)
	if err != nil {
		return nil, err
	}
	if j.Alg == "" {
		j.Alg = "HS256"
	}

	// A 部分
	method := jwt.GetSigningMethod(j.Alg)
	token := jwt.New(method)
	// B 部分
	claims := token.Claims.(jwt.MapClaims)
	if data != nil {
		for k, v := range data {
			claims[k] = v
		}
	}
	if j.expireFunc == nil {
		j.expireFunc = func() time.Time {
			return time.Now()
		}
	}
	claims["exp"] = j.expireFunc().Add(j.Expire).Unix()
	claims["iat"] = j.expireFunc().Unix()
	// C 部分
	var tokenString string
	if j.usingPublicKeyAlgorithm() {
		tokenString, err = token.SignedString(j.PrivateKeys)
	} else {
		tokenString, err = token.SignedString([]byte(j.Secret))
	}
	if err != nil {
		return nil, err
	}
	jr := &JwtResponse{
		Token: tokenString,
	}

	// RefreshToken
	refToken, err := j.refreshToken(token)
	if err != nil {
		return nil, err
	}
	jr.RefreshToken = refToken

	if j.SendCookie {
		if j.CookieName == "" {
			j.CookieName = "fes_token"
		}
		ctx.SetCookie(j.CookieName, tokenString, int(j.Expire.Seconds()), "/", "", false, true)
	}
	return jr, nil
}

func (j *JwtHandler) usingPublicKeyAlgorithm() bool {
	switch j.Alg {
	case "RS256", "RS384", "RS512", "ES256", "ES384", "ES512", "PS256", "PS384", "PS512":
		return true
	}
	return false
}

// refreshToken 获取刷新token
func (j *JwtHandler) refreshToken(token *jwt.Token) (tokenString string, err error) {
	claims := token.Claims.(jwt.MapClaims)
	claims["exp"] = j.expireFunc().Add(j.RefreshExpire).Unix()
	if j.usingPublicKeyAlgorithm() {
		tokenString, err = token.SignedString(j.PrivateKeys)
	} else {
		tokenString, err = token.SignedString([]byte(j.Secret))
	}
	return
}