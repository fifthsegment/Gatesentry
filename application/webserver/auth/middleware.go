package gatesentryWebserverAuth

// func InitAuthMiddleware(admin string, pass string) (context.Handler, *jwtmiddleware.Middleware) {
// 	authentication := basicauth.Default(map[string]string{
// 		admin: pass,
// 	})

// 	myJwtMiddleware := jwtmiddleware.New(jwtmiddleware.Config{
// 		ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
// 			return []byte(GSSigningKey), nil
// 		},
// 		SigningMethod: jwt.SigningMethodHS256,
// 	})
// 	return authentication, myJwtMiddleware
// }
