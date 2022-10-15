package Middlewares

import (
	"github.com/WangYihang/Platypus/internal/context/Conf"
	"github.com/dgrijalva/jwt-go"
	"time"
)

type Claims struct {
	UserName string
	jwt.StandardClaims
}

// CreateRefreshToken 把用户名加密为refresh token
func CreateRefreshToken(userName string) (string, error) {
	expirationTime := time.Now().Add(time.Duration(Conf.RestfulConf.RefreshExpireTime) * time.Second).Unix()
	claim := &Claims{
		UserName: userName,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime,
		},
	}
	//生成token jwt.NewWithClaims(签名算法。库中内置了常用的算法。最常用的是HS256;需要传递的信息)
	_token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	//进一步加密
	token, err := _token.SignedString([]byte(Conf.RestfulConf.JWTRefreshKey))
	return token, err
}

// CreateAccessToken 把用户名加密为access token
func CreateAccessToken(userName string) (string, error) {
	expirationTime := time.Now().Add(time.Duration(Conf.RestfulConf.AccessExpireTime) * time.Second).Unix()
	//expirationTime := time.Now().Add(Conf.AccessExpireTime).Unix()
	claim := &Claims{
		UserName: userName,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime,
		},
	}
	_token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	token, err := _token.SignedString([]byte(Conf.RestfulConf.JWTAccessKey))
	return token, err
}

// 把refresh token解密出用户名, 并验证是否过期
func verifyRefreshToken(refresh string) (string, bool) {
	claim := &Claims{}
	claimsInterface, err := jwt.ParseWithClaims(refresh, claim, func(token *jwt.Token) (interface{}, error) {
		return []byte(Conf.RestfulConf.JWTRefreshKey), nil
	})
	if err == nil && claimsInterface.Valid && claim.ExpiresAt >= time.Now().Unix() {
		return claim.UserName, true
	}
	return "", false
}

// 把access token解密出用户名, 并验证是否过期
func verifyAccessToken(ac string) (string, bool) {
	claim := &Claims{}
	claimsInterface, err := jwt.ParseWithClaims(ac, claim, func(token *jwt.Token) (interface{}, error) {
		return []byte(Conf.RestfulConf.JWTAccessKey), nil
	})
	if err == nil && claimsInterface.Valid && claim.ExpiresAt >= time.Now().Unix() {
		return claim.UserName, true
	}
	return "", false
}
