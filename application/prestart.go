package gatesentryf

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strings"
)

var Baseendpointv2 string

func PSSetApiBase(url string) {
	Baseendpointv2 = url
}

func PSGetMac() string {
	fmt.Println("Checking if API key exists")
	apifile := GSBASEDIR + "/" + ".api"

	if _, er := os.Stat(apifile); os.IsNotExist(er) {
		keystr := ""
		return keystr
	} else {
		filebytes, _ := ioutil.ReadFile(apifile)
		filestring := string(filebytes)
		fmt.Println("API: " + filestring)
		verified := verifyKey(filestring)
		if !verified {
			fmt.Println("Unable to verify key")
			os.Exit(1)
		}
		return filestring
		//os.Exit(1)
	}

	addrs, err := net.InterfaceAddrs()

	if err != nil {
		fmt.Println(err)
	}

	var currentIP, currentNetworkHardwareName string
	var goodIPs []string

	for _, address := range addrs {

		// check the address type and if it is not a loopback the display it
		// = GET LOCAL IP ADDRESS
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				// fmt.Println("Current IP address : ", ipnet.IP.String())
				currentIP = ipnet.IP.String()
				goodIPs = append(goodIPs, currentIP)
			}
		}
	}

	// get all the system's or local machine's network interfaces

	interfaces, _ := net.Interfaces()
	for _, interf := range interfaces {
		// fmt.Println(interf)
		if addrs, err := interf.Addrs(); err == nil {
			for _, addr := range addrs {
				// fmt.Println("[", index, "]", interf.Name, ">", addr)
				if macNotIn(interf.Name) {
					for i := 0; i < len(goodIPs); i++ {
						// only interested in the name with current IP address
						if strings.Contains(addr.String(), goodIPs[i]) {
							// fmt.Println("Use name : ", interf.Name)
							currentNetworkHardwareName = interf.Name
						}
					}

				}
				// currentNetworkHardwareName  = "";
			}
		}
	}

	// fmt.Println("------------------------------")

	// extract the hardware information base on the interface name
	// capture above
	netInterface, err := net.InterfaceByName(currentNetworkHardwareName)

	if err != nil {
		fmt.Println("Error: Unable to get device address, are you connected to the internet?")
		os.Exit(1)
	}

	_ = netInterface.Name
	macAddress := netInterface.HardwareAddr

	// fmt.Println("Hardware name : ", name)
	// fmt.Println("MAC address : ", macAddress)

	// verify if the MAC address can be parsed properly
	hwAddr, err := net.ParseMAC(macAddress.String())

	if err != nil {
		fmt.Println("Not able to parse MAC address : ", err)
		os.Exit(-1)
	}

	return hwAddr.String()
	// fmt.Printf("Physical hardware address : %s \n", hwAddr.String())
}

func macNotIn(check string) bool {
	check = strings.ToLower(check)
	if strings.Contains(check, "vmware") ||
		strings.Contains(check, "docker") ||
		strings.Contains(check, "virtualbox") ||
		strings.Contains(check, "tredo tunneling") ||
		strings.Contains(check, "microsoft") {
		return false
	}
	return true
}

func verifyKey(key string) bool {
	return true
	// key = strings.TrimSuffix(key, "\n");
	// fmt.Println("Verifying key = " + key)
	// url := Baseendpointv2 + "/verify/key"
	// client := &http.Client{}
	// req, _ := http.NewRequest("GET", url, nil)
	// req.Header.Set("X-API-KEY", key)
	// // res, _ := client.Do(req)
	// if res, err := client.Do(req); err != nil {
	//     // return nil, err
	//     fmt.Println(err)
	//     os.Exit(1)
	// } else {
	//     defer res.Body.Close()
	//     // return readBody(resp.Body)
	//     if res.StatusCode == http.StatusOK {
	//         bodyBytes, err2 := ioutil.ReadAll(res.Body)
	//         if (err2!=nil){
	//             fmt.Println("Unable to verify API key")
	//             os.Exit(1)
	//         }
	//         bodyString := string(bodyBytes)
	//         fmt.Println(bodyString)
	//     }else{
	//         fmt.Println("Error: Incorrect API key")
	//     }
	// }

}

func PSBuildInstallationIDFromMac(mac string) string {
	a := strings.ToUpper(strings.Replace(mac, ":", "", -1))
	lic := "GA" + a + "TE" + "SE" + a + "NT"
	return lic
}
