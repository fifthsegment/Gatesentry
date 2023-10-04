package gatesentryWebserverAuth

import (
	"github.com/dgrijalva/jwt-go"
	jwtmiddleware "github.com/iris-contrib/middleware/jwt"
	"github.com/kataras/iris/v12/context"
	"github.com/kataras/iris/v12/middleware/basicauth"
)

func InitAuthMiddleware(admin string, pass string) (context.Handler, *jwtmiddleware.Middleware) {
	authentication := basicauth.Default(map[string]string{
		admin: pass,
	})

	myJwtMiddleware := jwtmiddleware.New(jwtmiddleware.Config{
		ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
			return []byte(GSSigningKey), nil
		},
		SigningMethod: jwt.SigningMethodHS256,
	})
	return authentication, myJwtMiddleware
}
