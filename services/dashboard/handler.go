package main

import (
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
)

//go:embed ui.html
var dashboardHTML []byte

// CarState holds the most recent telemetry snapshot for one car.
// Sent over WebSocket to every connected browser client.
type CarState struct {
	CarID              string    `json:"car_id"`
	SessionID          string    `json:"session_id"`
	Lap                int       `json:"lap"`
	Sector             int       `json:"sector"`
	SpeedKmh           float64   `json:"speed_kmh"`
	RPM                float64   `json:"rpm"`
	ThrottlePercent    float64   `json:"throttle_percent"`
	BrakePressureBar   float64   `json:"brake_pressure_bar"`
	TireFlTemp         float64   `json:"tire_fl_temp"`
	TireFrTemp         float64   `json:"tire_fr_temp"`
	TireRlTemp         float64   `json:"tire_rl_temp"`
	TireRrTemp         float64   `json:"tire_rr_temp"`
	FuelLevelLiters    float64   `json:"fuel_level_liters"`
	FuelCapacityLiters float64   `json:"fuel_capacity_liters"`
	FuelFlowLph        float64   `json:"fuel_flow_lph"`
	BrakeTemp          float64   `json:"brake_temp"`
	InPit              bool      `json:"in_pit"`
	OnTrack            bool      `json:"on_track"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Broadcast is the JSON payload sent to browser clients every 200ms.
type Broadcast struct {
	Cars []CarState `json:"cars"`
}

// Hub manages the set of connected WebSocket clients and the shared car state map.
type Hub struct {
	carsMu sync.RWMutex
	cars   map[string]*CarState // keyed by car_id

	clientsMu sync.Mutex
	clients   map[*websocket.Conn]struct{}

	upgrader websocket.Upgrader
}

func newHub() *Hub {
	return &Hub{
		cars:    make(map[string]*CarState),
		clients: make(map[*websocket.Conn]struct{}),
		upgrader: websocket.Upgrader{
			// Allow all origins in dev; restrict in production.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// run broadcasts a snapshot of all car states to every WebSocket client
// at 200ms intervals. This is the only goroutine that writes to clients.
func (h *Hub) run() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		h.broadcast()
	}
}

// subscribeNATS listens on TELEMETRY.* and keeps the car state map current.
// Because NATS delivers each message in a separate goroutine, the map is
// protected by a RW mutex.
func (h *Hub) subscribeNATS(nc *nats.Conn) {
	_, err := nc.Subscribe("TELEMETRY.*", func(msg *nats.Msg) {
		state := parseCarState(msg.Data)
		if state == nil {
			return
		}
		h.carsMu.Lock()
		h.cars[state.CarID] = state
		h.carsMu.Unlock()
	})
	if err != nil {
		log.Fatalf("NATS subscribe: %v", err)
	}
	log.Println("Subscribed to TELEMETRY.*")
}

// broadcast serialises the current car map and writes it to every client.
// Clients that have closed their connection are removed from the set.
func (h *Hub) broadcast() {
	h.carsMu.RLock()
	states := make([]CarState, 0, len(h.cars))
	for _, s := range h.cars {
		states = append(states, *s)
	}
	h.carsMu.RUnlock()

	if len(states) == 0 {
		return
	}

	data, err := json.Marshal(Broadcast{Cars: states})
	if err != nil {
		return
	}

	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	for conn := range h.clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			conn.Close()
			delete(h.clients, conn)
		}
	}
}

// serveWS upgrades an HTTP connection to WebSocket and registers it with the hub.
func (h *Hub) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WS upgrade: %v", err)
		return
	}

	h.clientsMu.Lock()
	h.clients[conn] = struct{}{}
	h.clientsMu.Unlock()

	// Keep reading so we detect when the client disconnects.
	go func() {
		defer func() {
			h.clientsMu.Lock()
			delete(h.clients, conn)
			h.clientsMu.Unlock()
			conn.Close()
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

// serveDashboard serves the embedded HTML file.
func serveDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(dashboardHTML)
}

// parseCarState decodes a raw NATS message body into a CarState.
// Uses a generic map so the dashboard doesn't need to import the simulator's types.
func parseCarState(data []byte) *CarState {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	s := &CarState{
		// Fuel capacity isn't in the event — use the WEC-standard 75 L.
		// Replace with a config map if cars have different tank sizes.
		FuelCapacityLiters: 75.0,
		UpdatedAt:          time.Now(),
	}

	getString := func(key string) string {
		v, _ := raw[key].(string)
		return v
	}
	getFloat := func(key string) float64 {
		v, _ := raw[key].(float64)
		return v
	}
	getInt := func(key string) int {
		v, _ := raw[key].(float64)
		return int(v)
	}
	getBool := func(key string) bool {
		v, _ := raw[key].(bool)
		return v
	}

	s.CarID = getString("car_id")
	s.SessionID = getString("session_id")
	s.Lap = getInt("lap")
	s.Sector = getInt("sector")
	s.SpeedKmh = getFloat("speed_kmh")
	s.RPM = getFloat("rpm")
	s.ThrottlePercent = getFloat("throttle_percent")
	s.BrakePressureBar = getFloat("brake_pressure_bar")
	s.TireFlTemp = getFloat("tire_fl_temp")
	s.TireFrTemp = getFloat("tire_fr_temp")
	s.TireRlTemp = getFloat("tire_rl_temp")
	s.TireRrTemp = getFloat("tire_rr_temp")
	s.FuelLevelLiters = getFloat("fuel_level_liters")
	s.FuelFlowLph = getFloat("fuel_flow_lph")
	s.BrakeTemp = getFloat("brake_temp")
	s.InPit = getBool("in_pit")
	s.OnTrack = getBool("on_track")

	if s.CarID == "" {
		return nil // malformed event
	}
	return s
}
