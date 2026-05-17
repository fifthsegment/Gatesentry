package gatesentryf

import (
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"

	"encoding/json"
	"log"
	"sync"
	"time"
)

var GSConsumption *GSConsumptionContainer

type GSConsumptionContainer struct {
	Data         int64
	LastDataSent int64
	Mutex        *sync.Mutex
	FirstRun     bool
}

func GSGetConsumptionData(installId string) (int64, string, error) {
	log.Println("Obtaining consumption data ")
	gscon := GatesentryTypes.GSConsumptionUpdater{Id: installId}
	_, err := json.Marshal(gscon)
	if err != nil {
		// panic(err)
		return 0, "", err
	}
	return 0, gscon.AdditionalInfo, nil
}

func NewGSConsumptionContainer() *GSConsumptionContainer {

	gg := GSConsumptionContainer{Data: 0, Mutex: &sync.Mutex{}, FirstRun: true}
	return &gg
}

func (C *GSConsumptionContainer) ZeroOut(d int64) {
	C.Mutex.Lock()

	log.Println("Updating Consumption Data")
	C.Data = C.Data - d
	C.Mutex.Unlock()
}

func (C *GSConsumptionContainer) UpdateData(d int64) {
	C.Mutex.Lock()

	log.Println("Updating Consumption Data")
	C.Data += d
	C.Mutex.Unlock()
}

func (C *GSConsumptionContainer) GetData() int64 {
	return C.Data
}

func ConsumptionUpdater() {
	if R.GSConsumptionUpdaterRunning {
		log.Println("Consumption Updater is already running")
		return
	}
	log.Println("Creating a new Consumption container")
	GSConsumption = NewGSConsumptionContainer()
	R.GSConsumptionUpdaterRunning = true
	go func() {

		t := time.NewTicker(time.Second * CONSUMPTIONUPDATEINTERVAL)
		_ = t

	}()
}
