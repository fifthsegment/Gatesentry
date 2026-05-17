module gatesentrybin

go 1.24.0

toolchain go1.24.10

replace bitbucket.org/abdullah_irfan/gatesentryf => ./application

replace bitbucket.org/abdullah_irfan/gatesentryproxy => ./gatesentryproxy

require (
	bitbucket.org/abdullah_irfan/gatesentryf v0.0.0-00010101000000-000000000000
	bitbucket.org/abdullah_irfan/gatesentryproxy v0.0.0-00010101000000-000000000000
)

require (
	github.com/StackExchange/wmi v0.0.0-20190523213315-cbe66965904d // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/h2non/filetype v1.1.3 // indirect
	github.com/jpillora/s3 v1.1.4 // indirect
	github.com/smartystreets/assertions v1.2.0 // indirect
	golang.org/x/image v0.33.0 // indirect
	gopkg.in/elazarl/goproxy.v1 v1.0.0-20180725130230-947c36da3153 // indirect
)

require (
	github.com/antonholmquist/jason v1.0.0 // indirect
	github.com/badoux/checkmail v1.2.1 // indirect
	github.com/jpillora/overseer v1.1.6
	github.com/kardianos/service v1.2.0
	github.com/miekg/dns v1.1.43 // indirect
	github.com/oleksandr/bonjour v0.0.0-20210301155756-30f43c61b915 // indirect
	github.com/steakknife/devnull v0.0.0-20140623005216-3159330e33fb
	github.com/tidwall/btree v0.6.1 // indirect
	github.com/tidwall/buntdb v1.2.6 // indirect
	github.com/tidwall/gjson v1.9.3 // indirect
	github.com/tidwall/grect v0.1.2 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.0 // indirect
	github.com/tidwall/rtred v0.1.2 // indirect
	github.com/tidwall/tinyqueue v0.1.1 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
)
