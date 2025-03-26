package gatesentry2storage

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
)

const ENCRYPTIONKEY = "AES256Key-A23AS98BVM94PO3XSD10AA"

var GSBASEDIR = "./gatesentry"

type Store struct {
	Id            string
	Encrypted     bool
	Encryptionkey string
	Data          []byte
}

func SetBaseDir(dir string) {
	GSBASEDIR = dir
}

func NewStore(name string, encrypt bool) *Store {
	s := &Store{Id: name, Encrypted: encrypt, Encryptionkey: ENCRYPTIONKEY}
	s.Load()
	s.Persist()
	return s
}

func (s *Store) Load() {
	data, err := os.ReadFile(GSBASEDIR + s.Id)
	if err != nil {
		fmt.Println("Unable to load Settings file: " + s.Id + " . Will create a new one.")
		data = []byte{}
		return
	}
	mm := make(map[string]string)
	if len(data) > 0 {
		if err := json.Unmarshal(data, &mm); err != nil {
			fmt.Printf("Storage Error in MapStore Update: %s\n", err)
			return
		}
	}
	if mm["encrypted"] == "true" {
		data, err = Decrypt([]byte(mm["data"]), []byte(s.Encryptionkey))
		if err != nil {
			fmt.Println("Storage Error in MapStore Decrypt : " + err.Error())
			data = []byte{}
		}
		s.Data = data
	} else {
		s.Data = []byte(mm["data"])
	}

}

func (s *Store) Persist() {
	mm := make(map[string]string)
	mm["data"] = string(s.Data)
	if s.Encrypted {
		mm["encrypted"] = "true"
		encryptedData, err := Encrypt(s.Data, []byte(s.Encryptionkey))
		if err != nil {
			fmt.Println(err.Error())
			encryptedData = []byte{}
		}
		mm["data"] = string(encryptedData)
	} else {
		mm["encrypted"] = "false"
	}
	mapJson, _ := json.Marshal(mm)

	b := []byte(string(mapJson))
	os.WriteFile(GSBASEDIR+s.Id, b, 0644)
}

func (s *Store) Set(data []byte) {
	s.Data = data
	s.Persist()
}

func (s *Store) Get() []byte {
	return s.Data
}

type MapStore struct {
	BaseStore *Store
	Mutex     *sync.Mutex
}

func NewMapStore(name string, encrypt bool) *MapStore {
	s := &Store{Id: name, Encrypted: encrypt, Encryptionkey: ENCRYPTIONKEY}
	m := &MapStore{BaseStore: s, Mutex: &sync.Mutex{}}
	s.Load()
	return m
}

func (m *MapStore) Update(key string, value string) {
	// fmt.Println("Waiting to update " + value)
	m.Mutex.Lock()
	// fmt.Println("Running Update for " + key + " with: " + value)
	data := m.BaseStore.Get()
	mm := make(map[string]string)
	if len(data) > 0 {
		if err := json.Unmarshal(data, &mm); err != nil {
			fmt.Printf("Storage Error in MapStore Update: %s\n", err)
			return
		}
	}
	mm[key] = value
	mapJson, _ := json.Marshal(mm)
	b := []byte(string(mapJson))
	m.BaseStore.Set(b)
	m.Mutex.Unlock()
}

func (m *MapStore) Get(key string) string {
	data := m.BaseStore.Get()
	mm := make(map[string]string)
	if len(data) == 0 {
		return ""
	}
	if err := json.Unmarshal(data, &mm); err != nil {
		return ""
	}
	return mm[key]
}

/**
* Gets as integer
 */
func (m *MapStore) GetInt(key string) int {
	data := m.BaseStore.Get()
	mm := make(map[string]string)
	if len(data) == 0 {
		return 0
	}
	if err := json.Unmarshal(data, &mm); err != nil {
		return 0
	}
	i, err := strconv.Atoi(mm[key])
	if err != nil {
		return 0
	}
	return i
}

func (m *MapStore) SetDefault(key string, value string) {
	data := m.BaseStore.Get()
	mm := make(map[string]string)
	if len(data) == 0 {
		// The file doesn't exist yet
		mm[key] = value
	} else {
		if err := json.Unmarshal(data, &mm); err != nil {
			return
		}
		if _, ok := mm[key]; ok {
			return
		} else {
			mm[key] = value
		}
	}
	mapJson, _ := json.Marshal(mm)
	b := []byte(string(mapJson))
	m.BaseStore.Set(b)
}
