package contract

import (
"encoding/hex"
"encoding/json"
"log"
"net/http"
)

// StartRPCServer() launches the plugin's own HTTP server.
func (p *Plugin) StartRPCServer() {
addr := p.config.RPCAddress
if addr == "" {
addr = ":50010"
}
mux := http.NewServeMux()

// GET /v1/query/markets           -> list all markets
// GET /v1/query/markets?id=<hex>  -> fetch a single market by id
mux.HandleFunc("/v1/query/markets", func(w http.ResponseWriter, r *http.Request) {
idHex := r.URL.Query().Get("id")

if idHex != "" {
marketId, err := hex.DecodeString(idHex)
if err != nil {
http.Error(w, "invalid id: must be hex-encoded", http.StatusBadRequest)
return
}
resp, qErr := p.QueryState(0, &PluginStateReadRequest{
Keys: []*PluginKeyRead{
{QueryId: 1, Key: KeyForMarket(marketId)},
},
})
if qErr != nil {
http.Error(w, qErr.Msg, http.StatusInternalServerError)
return
}
if len(resp.Results) == 0 || len(resp.Results[0].Entries) == 0 {
http.Error(w, "market not found", http.StatusNotFound)
return
}
market := &MarketState{}
if err := Unmarshal(resp.Results[0].Entries[0].Value, market); err != nil {
http.Error(w, "failed to decode market", http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{
"id":     idHex,
"market": market,
})
return
}

resp, qErr := p.QueryState(0, &PluginStateReadRequest{
Ranges: []*PluginRangeRead{
{QueryId: 1, Prefix: JoinLenPrefix(marketPrefix), Limit: 0},
},
})
if qErr != nil {
http.Error(w, qErr.Msg, http.StatusInternalServerError)
return
}
type marketEntry struct {
Id     string       `json:"id"`
Market *MarketState `json:"market"`
}
var markets []marketEntry
if len(resp.Results) > 0 {
for _, entry := range resp.Results[0].Entries {
market := &MarketState{}
if err := Unmarshal(entry.Value, market); err != nil {
continue
}
markets = append(markets, marketEntry{Id: hex.EncodeToString(entry.Key), Market: market})
}
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(markets)
})

log.Printf("plugin RPC server listening on %s (routes: /v1/query/markets)", addr)
if err := http.ListenAndServe(addr, mux); err != nil {
log.Printf("plugin RPC server error: %v", err)
}
}
