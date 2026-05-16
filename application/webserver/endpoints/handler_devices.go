package gatesentryWebserverEndpoints

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"bitbucket.org/abdullah_irfan/gatesentryf/dns/discovery"
	gatesentryDnsServer "bitbucket.org/abdullah_irfan/gatesentryf/dns/server"
	"github.com/gorilla/mux"
)

// deviceStoreOrError returns the device store or writes a 503 error.
func deviceStoreOrError(w http.ResponseWriter) *discovery.DeviceStore {
	ds := gatesentryDnsServer.GetDeviceStore()
	if ds == nil {
		http.Error(w, `{"error":"Device store not initialized — DNS server may not be running"}`, http.StatusServiceUnavailable)
		return nil
	}
	return ds
}

// GSApiDevicesGetAll returns all devices in the inventory.
// GET /api/devices
func GSApiDevicesGetAll(w http.ResponseWriter, r *http.Request) {
	ds := deviceStoreOrError(w)
	if ds == nil {
		return
	}

	devices := ds.GetAllDevices()

	// Mark stale devices as offline (5-minute threshold)
	ds.MarkOffline(5 * time.Minute)

	// Re-fetch after marking offline so Online flags are current
	devices = ds.GetAllDevices()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"devices": devices,
		"count":   len(devices),
	})
}

// GSApiDeviceGet returns a single device by ID.
// GET /api/devices/{id}
func GSApiDeviceGet(w http.ResponseWriter, r *http.Request) {
	ds := deviceStoreOrError(w)
	if ds == nil {
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	device := ds.GetDevice(id)
	if device == nil {
		http.Error(w, `{"error":"Device not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device": device,
	})
}

// nameRequest is the JSON body for naming/updating a device.
type nameRequest struct {
	Name     string `json:"name"`
	Owner    string `json:"owner,omitempty"`
	Category string `json:"category,omitempty"`
}

// GSApiDeviceSetName sets the manual name (and optionally owner/category) for a device.
// POST /api/devices/{id}/name
func GSApiDeviceSetName(w http.ResponseWriter, r *http.Request) {
	ds := deviceStoreOrError(w)
	if ds == nil {
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	device := ds.GetDevice(id)
	if device == nil {
		http.Error(w, `{"error":"Device not found"}`, http.StatusNotFound)
		return
	}

	var req nameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"Invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	// Update the device via UpsertDevice to trigger DNS record rebuild
	device.ManualName = req.Name
	if req.Owner != "" {
		device.Owner = req.Owner
	}
	if req.Category != "" {
		device.Category = req.Category
	}
	device.Persistent = true // Named devices should survive restarts

	ds.UpsertDevice(device)

	log.Printf("[Devices API] Device %s named: %q (owner=%q, category=%q)", id, req.Name, req.Owner, req.Category)

	// Return updated device
	updated := ds.GetDevice(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"device": updated,
	})
}

// GSApiDeviceDelete removes a device from the inventory.
// DELETE /api/devices/{id}
func GSApiDeviceDelete(w http.ResponseWriter, r *http.Request) {
	ds := deviceStoreOrError(w)
	if ds == nil {
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]

	device := ds.GetDevice(id)
	if device == nil {
		http.Error(w, `{"error":"Device not found"}`, http.StatusNotFound)
		return
	}

	ds.RemoveDevice(id)

	log.Printf("[Devices API] Device %s (%s) removed", id, device.GetDisplayName())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Device removed",
	})
}
