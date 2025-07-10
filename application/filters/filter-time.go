package gatesentry2filters

import (
	"encoding/json"
	"log"
	"time"

	gatesentry2responder "bitbucket.org/abdullah_irfan/gatesentryf/responder"
)

// {"fromhours":21,"tohours":6,"fromminutes":30,"tominutes":45}

type GSBlockTimes struct {
	Fromhours   int `json:"fromhours"`
	Tohours     int `json:"tohours"`
	Fromminutes int `json:"fromminutes"`
	Tominutes   int `json:"tominutes"`
}

func GetTime(hour int, minute int, timezone string) time.Time {
	now := time.Now()
	loc, _ := time.LoadLocation(timezone)
	now = now.In(loc)
	n1 := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, now.Second(), now.Nanosecond(), now.Location())

	return n1
}

func RunTimeFilter(responder *gatesentry2responder.GSFilterResponder, blockedtimes string, timezone string) {

	t := time.Now()

	loc, erro := time.LoadLocation(timezone)
	if erro != nil {
		log.Printf("error loading location '%s': %v\n", timezone, erro)
		return
	}
	// log.Println("Location found in db = " + timezone)
	t = t.In(loc)
	blocktimes := GSBlockTimes{}
	// fmt.Println(blockedtimes)
	err := json.Unmarshal([]byte(blockedtimes), &blocktimes)
	// fmt.Println(err)
	if err != nil {
		// fmt.Println(err)
		return
	}
	fromTime := GetTime(blocktimes.Fromhours, blocktimes.Fromminutes, timezone)
	// fromTime = fromTime.In(loc)

	toTime := GetTime(blocktimes.Tohours, blocktimes.Tominutes, timezone)
	// toTime = toTime.In(loc)
	if toTime.Before(fromTime) {
		// log.Println("Holy Crap to time is before from time!");
		toTime = toTime.AddDate(0, 0, 1)
	}

	// fmt.Println( t )
	// fmt.Println( toTime )

	// fmt.Println(blocktimes)
	// fmt.Println(t.Hour());
	// fmt.Println(t.Minute())
	// fmt.Println("=============")
	// fmt.Println(blocktimes.Fromhours)
	// fmt.Println(blocktimes.Fromminutes)
	// (t.Hour() <= blocktimes.Tohours &&
	// t.Minute() <= blocktimes.Tominutes)
	// log.Println("Running time filter");
	if t.After(fromTime) {
		// log.Println("Current time is greater than block FROM Time");
		if t.Before(toTime) {
			// log.Println("Current time is less than block TO Time");
			responder.Blocked = true
		}
	}
	// before := time.Now();
	// time.Parse()
	// currentHour:= t.Hour();
	// currentMinute := t.Minute();
	// if ( currentHour >= blocktimes.Fromhours &&  currentMinute >= blocktimes.Fromminutes ){
	// 	log.Println("Current time is greater than block FROM Time");
	// 	if ( currentHour <= blocktimes.Tohours && currentMinute <= blocktimes.Tominutes ){
	// 		log.Println("This is a blocked time!");
	// 		responder.Blocked = true;
	// 	}
	// }

	// fmt.Println( blocktimes );
}
