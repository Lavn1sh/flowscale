package api

import (
	"encoding/json"
	"net/http"
	"os"
)

const shipmentDownFile = "shipment_down.txt"

type DemoHandler struct{}

func NewDemoHandler() *DemoHandler {
	return &DemoHandler{}
}

func (h *DemoHandler) GetShipmentStatus(w http.ResponseWriter, r *http.Request) {
	_, err := os.Stat(shipmentDownFile)
	isDown := !os.IsNotExist(err)

	json.NewEncoder(w).Encode(map[string]bool{"down": isDown})
}

func (h *DemoHandler) SetShipmentStatus(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Down bool `json:"down"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Down {
		// Create the file
		file, err := os.Create(shipmentDownFile)
		if err == nil {
			file.Close()
		}
	} else {
		// Remove the file
		os.Remove(shipmentDownFile)
	}

	json.NewEncoder(w).Encode(map[string]bool{"down": req.Down})
}

func (h *DemoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		h.GetShipmentStatus(w, r)
	case http.MethodPost:
		h.SetShipmentStatus(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
