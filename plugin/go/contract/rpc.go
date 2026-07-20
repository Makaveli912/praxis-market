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

	// GET /v1/query/proposals             -> list all proposals
	// GET /v1/query/proposals?market=<hex> -> fetch a single proposal by market id
	mux.HandleFunc("/v1/query/proposals", func(w http.ResponseWriter, r *http.Request) {
		marketHex := r.URL.Query().Get("market")

		if marketHex != "" {
			marketId, err := hex.DecodeString(marketHex)
			if err != nil {
				http.Error(w, "invalid market: must be hex-encoded", http.StatusBadRequest)
				return
			}
			resp, qErr := p.QueryState(0, &PluginStateReadRequest{
				Keys: []*PluginKeyRead{
					{QueryId: 1, Key: KeyForProposal(marketId)},
				},
			})
			if qErr != nil {
				http.Error(w, qErr.Msg, http.StatusInternalServerError)
				return
			}
			if len(resp.Results) == 0 || len(resp.Results[0].Entries) == 0 {
				http.Error(w, "proposal not found", http.StatusNotFound)
				return
			}
			proposal := &ProposalRecord{}
			if err := Unmarshal(resp.Results[0].Entries[0].Value, proposal); err != nil {
				http.Error(w, "failed to decode proposal", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"market":   marketHex,
				"proposal": proposal,
			})
			return
		}

		resp, qErr := p.QueryState(0, &PluginStateReadRequest{
			Ranges: []*PluginRangeRead{
				{QueryId: 1, Prefix: JoinLenPrefix(proposalPrefix), Limit: 0},
			},
		})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}
		type proposalEntry struct {
			Market   string          `json:"market"`
			Proposal *ProposalRecord `json:"proposal"`
		}
		var proposals []proposalEntry
		if len(resp.Results) > 0 {
			for _, entry := range resp.Results[0].Entries {
				proposal := &ProposalRecord{}
				if err := Unmarshal(entry.Value, proposal); err != nil {
					continue
				}
				rawKey := entry.Key
				marketIdBytes := rawKey
				if len(rawKey) > 20 {
					marketIdBytes = rawKey[len(rawKey)-20:]
				}
				proposals = append(proposals, proposalEntry{
					Market: hex.EncodeToString(marketIdBytes), Proposal: proposal,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(proposals)
	})

	// GET /v1/query/disputes             -> list all disputes
	// GET /v1/query/disputes?market=<hex> -> fetch a single dispute by market id
	mux.HandleFunc("/v1/query/disputes", func(w http.ResponseWriter, r *http.Request) {
		marketHex := r.URL.Query().Get("market")

		if marketHex != "" {
			marketId, err := hex.DecodeString(marketHex)
			if err != nil {
				http.Error(w, "invalid market: must be hex-encoded", http.StatusBadRequest)
				return
			}
			resp, qErr := p.QueryState(0, &PluginStateReadRequest{
				Keys: []*PluginKeyRead{
					{QueryId: 1, Key: KeyForDispute(marketId)},
				},
			})
			if qErr != nil {
				http.Error(w, qErr.Msg, http.StatusInternalServerError)
				return
			}
			if len(resp.Results) == 0 || len(resp.Results[0].Entries) == 0 {
				http.Error(w, "dispute not found", http.StatusNotFound)
				return
			}
			dispute := &DisputeRecord{}
			if err := Unmarshal(resp.Results[0].Entries[0].Value, dispute); err != nil {
				http.Error(w, "failed to decode dispute", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"market":  marketHex,
				"dispute": dispute,
			})
			return
		}

		resp, qErr := p.QueryState(0, &PluginStateReadRequest{
			Ranges: []*PluginRangeRead{
				{QueryId: 1, Prefix: JoinLenPrefix(disputePrefix), Limit: 0},
			},
		})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}
		type disputeEntry struct {
			Market  string         `json:"market"`
			Dispute *DisputeRecord `json:"dispute"`
		}
		var disputes []disputeEntry
		if len(resp.Results) > 0 {
			for _, entry := range resp.Results[0].Entries {
				dispute := &DisputeRecord{}
				if err := Unmarshal(entry.Value, dispute); err != nil {
					continue
				}
				rawKey := entry.Key
				marketIdBytes := rawKey
				if len(rawKey) > 20 {
					marketIdBytes = rawKey[len(rawKey)-20:]
				}
				disputes = append(disputes, disputeEntry{
					Market: hex.EncodeToString(marketIdBytes), Dispute: dispute,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(disputes)
	})

	// GET /v1/query/votes?market=<hex>  -> panel vote status for a market (commit+reveal merged per voter)
	mux.HandleFunc("/v1/query/votes", func(w http.ResponseWriter, r *http.Request) {
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
				{QueryId: 1, Prefix: JoinLenPrefix(voteCommitPrefix), Limit: 0},
				{QueryId: 2, Prefix: JoinLenPrefix(voteRevealPrefix), Limit: 0},
			},
		})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}

		type voteStatus struct {
			Voter       string `json:"voter"`
			Committed   bool   `json:"committed"`
			CommitHash  string `json:"commitHash,omitempty"`
			CommittedAt uint64 `json:"committedAt,omitempty"`
			Revealed    bool   `json:"revealed"`
			Vote        bool   `json:"vote,omitempty"`
			RevealedAt  uint64 `json:"revealedAt,omitempty"`
		}
		byVoter := map[string]*voteStatus{}

		for _, res := range resp.Results {
			for _, entry := range res.Entries {
				rawKey := entry.Key
				if len(rawKey) < 40 {
					continue
				}
				composite := rawKey[len(rawKey)-40:]
				entryMarketId := composite[:20]
				voterAddr := composite[20:]
				if !bytesEqual(entryMarketId, marketId) {
					continue
				}
				voterHex := hex.EncodeToString(voterAddr)
				if byVoter[voterHex] == nil {
					byVoter[voterHex] = &voteStatus{Voter: voterHex}
				}
				if res.QueryId == 1 {
					vc := &VoteCommit{}
					if perr := Unmarshal(entry.Value, vc); perr == nil {
						byVoter[voterHex].Committed = true
						byVoter[voterHex].CommitHash = hex.EncodeToString(vc.CommitHash)
						byVoter[voterHex].CommittedAt = vc.CommittedAt
					}
				} else if res.QueryId == 2 {
					vr := &VoteReveal{}
					if perr := Unmarshal(entry.Value, vr); perr == nil {
						byVoter[voterHex].Revealed = true
						byVoter[voterHex].Vote = vr.Vote
						byVoter[voterHex].RevealedAt = vr.RevealedAt
					}
				}
			}
		}

		var votes []*voteStatus
		for _, v := range byVoter {
			votes = append(votes, v)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(votes)
	})

	// GET /v1/query/outcomes             -> list all resolved outcomes
	// GET /v1/query/outcomes?market=<hex> -> fetch a single outcome by market id
	mux.HandleFunc("/v1/query/outcomes", func(w http.ResponseWriter, r *http.Request) {
		marketHex := r.URL.Query().Get("market")

		if marketHex != "" {
			marketId, err := hex.DecodeString(marketHex)
			if err != nil {
				http.Error(w, "invalid market: must be hex-encoded", http.StatusBadRequest)
				return
			}
			resp, qErr := p.QueryState(0, &PluginStateReadRequest{
				Keys: []*PluginKeyRead{
					{QueryId: 1, Key: KeyForOutcome(marketId)},
				},
			})
			if qErr != nil {
				http.Error(w, qErr.Msg, http.StatusInternalServerError)
				return
			}
			if len(resp.Results) == 0 || len(resp.Results[0].Entries) == 0 {
				http.Error(w, "outcome not found", http.StatusNotFound)
				return
			}
			outcome := &OutcomeState{}
			if err := Unmarshal(resp.Results[0].Entries[0].Value, outcome); err != nil {
				http.Error(w, "failed to decode outcome", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"market":  marketHex,
				"outcome": outcome,
			})
			return
		}

		resp, qErr := p.QueryState(0, &PluginStateReadRequest{
			Ranges: []*PluginRangeRead{
				{QueryId: 1, Prefix: JoinLenPrefix(outcomePrefix), Limit: 0},
			},
		})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}
		type outcomeEntry struct {
			Market  string        `json:"market"`
			Outcome *OutcomeState `json:"outcome"`
		}
		var outcomes []outcomeEntry
		if len(resp.Results) > 0 {
			for _, entry := range resp.Results[0].Entries {
				outcome := &OutcomeState{}
				if err := Unmarshal(entry.Value, outcome); err != nil {
					continue
				}
				rawKey := entry.Key
				marketIdBytes := rawKey
				if len(rawKey) > 20 {
					marketIdBytes = rawKey[len(rawKey)-20:]
				}
				outcomes = append(outcomes, outcomeEntry{
					Market: hex.EncodeToString(marketIdBytes), Outcome: outcome,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(outcomes)
	})

	// GET /v1/query/slashes                    -> list all slash records
	// GET /v1/query/slashes?address=<hex>      -> fetch a single slash record by resolver address
	mux.HandleFunc("/v1/query/slashes", func(w http.ResponseWriter, r *http.Request) {
		addrHex := r.URL.Query().Get("address")

		if addrHex != "" {
			addr, err := hex.DecodeString(addrHex)
			if err != nil {
				http.Error(w, "invalid address: must be hex-encoded", http.StatusBadRequest)
				return
			}
			resp, qErr := p.QueryState(0, &PluginStateReadRequest{
				Keys: []*PluginKeyRead{
					{QueryId: 1, Key: KeyForSlashRecord(addr)},
				},
			})
			if qErr != nil {
				http.Error(w, qErr.Msg, http.StatusInternalServerError)
				return
			}
			if len(resp.Results) == 0 || len(resp.Results[0].Entries) == 0 {
				http.Error(w, "slash record not found", http.StatusNotFound)
				return
			}
			slash := &SlashRecord{}
			if err := Unmarshal(resp.Results[0].Entries[0].Value, slash); err != nil {
				http.Error(w, "failed to decode slash record", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"address": addrHex,
				"slash":   slash,
			})
			return
		}

		resp, qErr := p.QueryState(0, &PluginStateReadRequest{
			Ranges: []*PluginRangeRead{
				{QueryId: 1, Prefix: JoinLenPrefix(slashRecordPrefix), Limit: 0},
			},
		})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}
		type slashEntry struct {
			Address string       `json:"address"`
			Slash   *SlashRecord `json:"slash"`
		}
		var slashes []slashEntry
		if len(resp.Results) > 0 {
			for _, entry := range resp.Results[0].Entries {
				slash := &SlashRecord{}
				if err := Unmarshal(entry.Value, slash); err != nil {
					continue
				}
				rawKey := entry.Key
				addrBytes := rawKey
				if len(rawKey) > 20 {
					addrBytes = rawKey[len(rawKey)-20:]
				}
				slashes = append(slashes, slashEntry{
					Address: hex.EncodeToString(addrBytes), Slash: slash,
				})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(slashes)
	})

	log.Printf("plugin RPC server listening on %s (routes: /v1/query/markets, /v1/query/positions, /v1/query/resolvers, /v1/query/proposals, /v1/query/disputes, /v1/query/votes, /v1/query/outcomes, /v1/query/slashes)", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("plugin RPC server error: %v", err)
	}
}
