package contract

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sort"
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
				// entry.Key = state-key prefix + 20-byte marketId (see KeyForMarket).
				// Only the trailing 20 bytes are the actual market ID.
				rawKey := entry.Key
				marketIdBytes := rawKey
				if len(rawKey) > 20 {
					marketIdBytes = rawKey[len(rawKey)-20:]
				}
				markets = append(markets, marketEntry{Id: hex.EncodeToString(marketIdBytes), Market: market})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(markets)
	})

// GET /v1/query/positions?market=<hex>  -> top holders for a market, sorted by total stake desc, capped at 10
mux.HandleFunc("/v1/query/positions", func(w http.ResponseWriter, r *http.Request) {
marketHex := r.URL.Query().Get("market")
if marketHex == "" {
http.Error(w, "missing required query param: market", http.StatusBadRequest)
return
}
marketId, err := hex.DecodeString(marketHex)
if err != nil {
http.Error(w, "invalid market: must be hex-encoded", http.StatusBadRequest)
return
}

resp, qErr := p.QueryState(0, &PluginStateReadRequest{
Ranges: []*PluginRangeRead{
{QueryId: 1, Prefix: JoinLenPrefix(positionPrefix), Limit: 0},
},
})
if qErr != nil {
http.Error(w, qErr.Msg, http.StatusInternalServerError)
return
}

type holderEntry struct {
Address   string `json:"address"`
SharesYes uint64 `json:"sharesYes"`
SharesNo  uint64 `json:"sharesNo"`
CostPaid  uint64 `json:"costPaid"`
Claimed   bool   `json:"claimed"`
}
var holders []holderEntry
if len(resp.Results) > 0 {
for _, entry := range resp.Results[0].Entries {
// entry.Key trailing 40 bytes = marketId(20) + address(20), per KeyForPosition.
rawKey := entry.Key
if len(rawKey) < 40 {
continue
}
composite := rawKey[len(rawKey)-40:]
entryMarketId := composite[:20]
addr := composite[20:]
if !bytesEqual(entryMarketId, marketId) {
continue
}
pos := &PositionState{}
if perr := Unmarshal(entry.Value, pos); perr != nil {
continue
}
holders = append(holders, holderEntry{
Address:   hex.EncodeToString(addr),
SharesYes: pos.SharesYes,
SharesNo:  pos.SharesNo,
CostPaid:  pos.CostPaid,
Claimed:   pos.Claimed,
})
}
}

sort.Slice(holders, func(i, j int) bool {
return (holders[i].SharesYes + holders[i].SharesNo) > (holders[j].SharesYes + holders[j].SharesNo)
})
if len(holders) > 10 {
holders = holders[:10]
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(holders)
})

// GET /v1/query/resolvers  -> list all registered resolvers
mux.HandleFunc("/v1/query/resolvers", func(w http.ResponseWriter, r *http.Request) {
idxResp, qErr := p.QueryState(0, &PluginStateReadRequest{
Keys: []*PluginKeyRead{
{QueryId: 1, Key: KeyForResolverIndex()},
},
})
if qErr != nil {
http.Error(w, qErr.Msg, http.StatusInternalServerError)
return
}
if idxResp.Error != nil {
http.Error(w, idxResp.Error.Msg, http.StatusInternalServerError)
return
}

var resolvers []*ResolverRecord
if len(idxResp.Results) > 0 && len(idxResp.Results[0].Entries) > 0 {
idx := &ResolverIndex{}
if perr := Unmarshal(idxResp.Results[0].Entries[0].Value, idx); perr == nil {
var reads []*PluginKeyRead
for i, addr := range idx.Addresses {
reads = append(reads, &PluginKeyRead{QueryId: uint64(i + 1), Key: KeyForResolverRecord(addr)})
}
if len(reads) > 0 {
recResp, qErr2 := p.QueryState(0, &PluginStateReadRequest{Keys: reads})
if qErr2 == nil && recResp.Error == nil {
for _, res := range recResp.Results {
if len(res.Entries) == 0 {
continue
}
rec := &ResolverRecord{}
if perr := Unmarshal(res.Entries[0].Value, rec); perr == nil {
resolvers = append(resolvers, rec)
}
}
}
}
}
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(resolvers)
})

	log.Printf("plugin RPC server listening on %s (routes: /v1/query/markets, /v1/query/positions, /v1/query/resolvers)", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("plugin RPC server error: %v", err)
	}
}
