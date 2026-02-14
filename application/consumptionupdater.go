package gatesentryf

import (
	GatesentryTypes "bitbucket.org/abdullah_irfan/gatesentryf/types"

	// "fmt"
	"encoding/json"
	"log"
	"sync"
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
	// data := gsconj
	return 0, gscon.AdditionalInfo, nil
	// _=resp;
}

func NewGSConsumptionContainer() *GSConsumptionContainer {

	gg := GSConsumptionContainer{Data: 0, Mutex: &sync.Mutex{}, FirstRun: true}
	return &gg
}

func (C *GSConsumptionContainer) ZeroOut(d int64) {
	// if (C.Mutex == nil ){
	// 	log.Println("ConsumptionUpdater: Mutex is nil, setting it up.")
	// 	C.Mutex = &sync.Mutex{};
	// }
	C.Mutex.Lock()

	log.Println("Updating Consumption Data")
	C.Data = C.Data - d
	C.Mutex.Unlock()
}

func (C *GSConsumptionContainer) UpdateData(d int64) {
	// if (C.Mutex == nil ){
	// 	log.Println("ConsumptionUpdater: Mutex is nil, setting it up.")
	// 	C.Mutex = &sync.Mutex{};
	// }
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
}
