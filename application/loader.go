package gatesentryf

import (
	"fmt"
)

func LoadFilters() {
	// for k, v := range R.FilterFiles {
	//     // v is the location of the file
	//     LoadFilter( v , k )
	//     _=k;
	//     _=v;
	// }
}

func RestartGS() {
	R.init()
	fmt.Println("-------------Restarting GateSentry-------------")
}
