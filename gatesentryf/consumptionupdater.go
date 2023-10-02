package gatesentryf

import (
	gscommonweb "bitbucket.org/abdullah_irfan/gatesentryf/commonweb"
	gstransport "bitbucket.org/abdullah_irfan/gatesentryf/securetransport"

	// "fmt"
	"encoding/json"
	"errors"
	"log"
	"strconv"
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
	gscon := gscommonweb.GSConsumptionUpdater{Id: installId}
	gsconj, err := json.Marshal(gscon)
	if err != nil {
		// panic(err)
		return 0, "", err
	}
	data := gsconj
	resp, err := gstransport.SendEncryptedData("/license/consumption/get", data, installId)

	if err != nil {
		// fmt.Println( err )
		return 0, "", err
	}
	json.Unmarshal([]byte(resp), &gscon)
	log.Println("Previous Consumption data = " + gscon.Message)
	i, err := strconv.ParseInt(gscon.Message, 10, 64)
	if err != nil {
		return 0, "", err
	}
	return i, gscon.AdditionalInfo, nil
	// _=resp;
}

func NewGSConsumptionContainer() *GSConsumptionContainer {
	// load data from api and file
	// dd, _, err := GSGetConsumptionData(INSTALLATIONID)
	// if err != nil {
	// 	log.Println( err )
	// }
	gg := GSConsumptionContainer{Data: 0, Mutex: &sync.Mutex{}, FirstRun: true}
	// gg.Mutex = &sync.Mutex{};
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

func PushConsumptionDataToServer(consumption int64) error {
	log.Println("Updating Consumption data")
	currenttime := time.Now().UnixNano() / 1000000000
	currenttimestring := strconv.FormatInt(currenttime, 10)
	gscon := gscommonweb.GSConsumptionUpdater{Id: INSTALLATIONID, Consumption: consumption, Time: currenttimestring}
	gsconj, err := json.Marshal(gscon)
	log.Println("Unable to marhsal consumption data")
	if err != nil {
		// panic(err)
		return err
	}
	data := gsconj
	log.Println("Pushing consumption data")
	resp, err := gstransport.SendEncryptedData("/license/consumption/update", data, INSTALLATIONID)

	if err != nil {
		R.FailedConsumptionUpdates++

		log.Println("Unable to update consumption data. " + err.Error())
		return err
	}
	responder := gscommonweb.GSConsumptionUpdaterResponse{Ok: false}
	json.Unmarshal([]byte(resp), &responder)
	log.Println("Consumption update response = ", resp)
	if !responder.Ok {
		log.Println("Response from server is not an Okay")
		R.FailedConsumptionUpdates++
		return errors.New("Unable to get an Okay from the main server.")
	} else {
		log.Println("Response from server is an Okay")
		R.FailedConsumptionUpdates = 0
	}

	log.Println((resp))
	_ = resp
	return nil
}

func ConsumptionUpdater() {
	if R.GSConsumptionUpdaterRunning {
		log.Println("Consumption Updater is already running")
		return
	}
	gstransport.SetbaseEndpoint(GSAPIBASEPOINT)
	log.Println("Creating a new Consumption container")
	GSConsumption = NewGSConsumptionContainer()
	// fmt.Println( )
	R.GSConsumptionUpdaterRunning = true
	go func() {

		t := time.NewTicker(time.Second * CONSUMPTIONUPDATEINTERVAL)
		_ = t
		// for{
		// 	dd := GSConsumption.GetData()
		// 	GSConsumption.LastDataSent = dd;
		// 	err := PushConsumptionDataToServer(dd)
		// 	if ( err == nil ){
		// 		GSConsumption.ZeroOut(dd)
		// 	}
		// 	<-t.C
		// }
	}()
}

func (R *GSRuntime) UpdateConsumption(consumption int64) {
	return
	go func() {
		GSConsumption.UpdateData(consumption)
	}()
}
