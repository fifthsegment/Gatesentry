package gatesentryWebserverEndpoints

import (
	"log"
	"time"

	gatesentryWebserverAuth "bitbucket.org/abdullah_irfan/gatesentryf/webserver/auth"
	gatesentryWebserverTypes "bitbucket.org/abdullah_irfan/gatesentryf/webserver/types"
	"github.com/dgrijalva/jwt-go"
	"github.com/kataras/iris/v12"
)

func GSGetToken(ctx iris.Context) {
	var user gatesentryWebserverTypes.Login

	if err := ctx.ReadJSON(&user); err != nil {
		log.Println(err.Error())
	}
	if gatesentryWebserverAuth.VerifyAdminUser(user.Username, user.Pass, settingsStore) {
		token := jwt.New(jwt.SigningMethodHS256)
		// Headers
		token.Header["alg"] = "HS256"
		token.Header["typ"] = "JWT"

		// Claims
		claims := token.Claims.(jwt.MapClaims)
		claims["name"] = user.Username
		claims["mail"] = user.Username
		claims["exp"] = time.Now().Add(time.Second * 3600).Unix()
		tokenString, err := token.SignedString([]byte(gatesentryWebserverAuth.GSSigningKey))
		if err != nil {
			ctx.JSON(struct {
				Validated bool
				Jwtoken   string
				Message   string
			}{Validated: false, Jwtoken: "", Message: "Unable to generate Token string because " + err.Error()})
		} else {
			ctx.JSON(struct {
				Validated bool
				Jwtoken   string
				Message   string
			}{Validated: true, Jwtoken: tokenString, Message: ""})
		}

	} else {
		ctx.JSON(struct {
			Validated bool
			Jwtoken   string
			Message   string
		}{Validated: false, Jwtoken: "", Message: "Unable to validate username password"})
	}
}
