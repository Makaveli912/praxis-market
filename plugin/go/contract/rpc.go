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

	// GET /v1/query/position?market=<hex>&address=<hex>  -> a single address's position in a market, plus market context
	mux.HandleFunc("/v1/query/position", func(w http.ResponseWriter, r *http.Request) {
		marketHex := r.URL.Query().Get("market")
		addrHex := r.URL.Query().Get("address")
		if marketHex == "" || addrHex == "" {
			http.Error(w, "missing required query params: market, address", http.StatusBadRequest)
			return
		}
		marketId, err := hex.DecodeString(marketHex)
		if err != nil {
			http.Error(w, "invalid market: must be hex-encoded", http.StatusBadRequest)
			return
		}
		addr, err := hex.DecodeString(addrHex)
		if err != nil {
			http.Error(w, "invalid address: must be hex-encoded", http.StatusBadRequest)
			return
		}

		resp, qErr := p.QueryState(0, &PluginStateReadRequest{
			Keys: []*PluginKeyRead{
				{QueryId: 1, Key: KeyForPosition(marketId, addr)},
				{QueryId: 2, Key: KeyForMarket(marketId)},
			},
		})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}

		result := map[string]interface{}{"market": marketHex, "address": addrHex}

		getEntry := func(qid uint64) []byte {
			for _, res := range resp.Results {
				if res.QueryId == qid && len(res.Entries) > 0 {
					return res.Entries[0].Value
				}
			}
			return nil
		}

		if v := getEntry(1); v != nil {
			pos := &PositionState{}
			if err := Unmarshal(v, pos); err == nil {
				result["position"] = pos
			}
		} else {
			result["position"] = nil
		}

		if v := getEntry(2); v != nil {
			mkt := &MarketState{}
			if err := Unmarshal(v, mkt); err == nil {
				result["market_state"] = mkt
			}
		} else {
			http.Error(w, "market not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// GET /v1/query/account?address=<hex>  -> built-in Account balance/vesting record
	mux.HandleFunc("/v1/query/account", func(w http.ResponseWriter, r *http.Request) {
		addrHex := r.URL.Query().Get("address")
		if addrHex == "" {
			http.Error(w, "missing required query param: address", http.StatusBadRequest)
			return
		}
		addr, err := hex.DecodeString(addrHex)
		if err != nil {
			http.Error(w, "invalid address: must be hex-encoded", http.StatusBadRequest)
			return
		}

		resp, qErr := p.QueryState(0, &PluginStateReadRequest{
			Keys: []*PluginKeyRead{
				{QueryId: 1, Key: KeyForAccount(addr)},
			},
		})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}
		if len(resp.Results) == 0 || len(resp.Results[0].Entries) == 0 {
			http.Error(w, "account not found", http.StatusNotFound)
			return
		}
		acct := &Account{}
		if err := Unmarshal(resp.Results[0].Entries[0].Value, acct); err != nil {
			http.Error(w, "failed to decode account", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"address": addrHex,
			"account": acct,
		})
	})

	// GET /v1/query/unbonding?address=<hex>  -> per-resolver unbonding status (0x29)
	mux.HandleFunc("/v1/query/unbonding", func(w http.ResponseWriter, r *http.Request) {
		addrHex := r.URL.Query().Get("address")
		if addrHex == "" {
			http.Error(w, "missing required query param: address", http.StatusBadRequest)
			return
		}
		addr, err := hex.DecodeString(addrHex)
		if err != nil {
			http.Error(w, "invalid address: must be hex-encoded", http.StatusBadRequest)
			return
		}

		resp, qErr := p.QueryState(0, &PluginStateReadRequest{
			Keys: []*PluginKeyRead{
				{QueryId: 1, Key: KeyForUnbondingRecord(addr)},
			},
		})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}
		if len(resp.Results) == 0 || len(resp.Results[0].Entries) == 0 {
			http.Error(w, "no unbonding record for this address", http.StatusNotFound)
			return
		}
		rec := &ResolverRecord{}
		if err := Unmarshal(resp.Results[0].Entries[0].Value, rec); err != nil {
			http.Error(w, "failed to decode unbonding record", http.StatusInternalServerError)
			return
		}
		now := GetGlobalHeight()
		result := map[string]interface{}{
			"address":                  addrHex,
			"unbonding_amount":         rec.UnbondingAmount,
			"unbonding_release_height": rec.UnbondingReleaseHeight,
			"current_height":           now,
			"released":                 now > 0 && now >= rec.UnbondingReleaseHeight,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	// GET /v1/query/dispute-context?market=<hex>&address=<hex>  -> proposal, dispute, outcome, caller position,
	// dispute-window timing, and a real should_dispute signal (not a placeholder).
	mux.HandleFunc("/v1/query/dispute-context", func(w http.ResponseWriter, r *http.Request) {
		marketHex := r.URL.Query().Get("market")
		addrHex := r.URL.Query().Get("address")
		if marketHex == "" {
			http.Error(w, "missing required query param: market", http.StatusBadRequest)
			return
		}
		marketId, err := hex.DecodeString(marketHex)
		if err != nil {
			http.Error(w, "invalid market: must be hex-encoded", http.StatusBadRequest)
			return
		}

		reads := []*PluginKeyRead{
			{QueryId: 1, Key: KeyForMarket(marketId)},
			{QueryId: 2, Key: KeyForProposal(marketId)},
			{QueryId: 3, Key: KeyForDispute(marketId)},
			{QueryId: 4, Key: KeyForOutcome(marketId)},
		}
		var addr []byte
		if addrHex != "" {
			addr, err = hex.DecodeString(addrHex)
			if err != nil {
				http.Error(w, "invalid address: must be hex-encoded", http.StatusBadRequest)
				return
			}
			reads = append(reads, &PluginKeyRead{QueryId: 5, Key: KeyForPosition(marketId, addr)})
		}

		resp, qErr := p.QueryState(0, &PluginStateReadRequest{Keys: reads})
		if qErr != nil {
			http.Error(w, qErr.Msg, http.StatusInternalServerError)
			return
		}

		getEntry := func(qid uint64) []byte {
			for _, res := range resp.Results {
				if res.QueryId == qid && len(res.Entries) > 0 {
					return res.Entries[0].Value
				}
			}
			return nil
		}

		if v := getEntry(1); v == nil {
			http.Error(w, "market not found", http.StatusNotFound)
			return
		}

		mkt := &MarketState{}
		Unmarshal(getEntry(1), mkt)

		result := map[string]interface{}{
			"market":      marketHex,
			"status":      mkt.Status,
			"expiry_time": mkt.ExpiryTime,
			"open_time":   mkt.OpenTime,
			"question":    mkt.Question,
		}

		var proposal *ProposalRecord
		hasProposal := false
		if v := getEntry(2); v != nil {
			proposal = &ProposalRecord{}
			if err := Unmarshal(v, proposal); err == nil {
				hasProposal = true
				result["proposal"] = map[string]interface{}{
					"resolver_addr":    hex.EncodeToString(proposal.ResolverAddr),
					"proposed_outcome": proposal.ProposedOutcome,
					"proposal_bond":    proposal.ProposalBond,
					"proposal_block":   proposal.ProposalBlock,
					"status":           proposal.Status,
				}
			}
		}

		var dispute *DisputeRecord
		hasDispute := false
		if v := getEntry(3); v != nil {
			dispute = &DisputeRecord{}
			if err := Unmarshal(v, dispute); err == nil {
				hasDispute = true
				var panelMembers []string
				for _, m := range dispute.PanelMembers {
					panelMembers = append(panelMembers, hex.EncodeToString(m))
				}
				result["dispute"] = map[string]interface{}{
					"disputer_address": hex.EncodeToString(dispute.DisputerAddress),
					"dispute_bond":     dispute.DisputeBond,
					"dispute_block":    dispute.DisputeBlock,
					"vote_status":      dispute.VoteStatus,
					"panel_size":       dispute.PanelSize,
					"panel_members":    panelMembers,
				}
			}
		}

		if v := getEntry(4); v != nil {
			outcome := &OutcomeState{}
			if err := Unmarshal(v, outcome); err == nil {
				result["outcome"] = map[string]interface{}{
					"winning_outcome": outcome.WinningOutcome,
					"resolved_at":     outcome.ResolvedAt,
				}
			}
		}

		var pos *PositionState
		hasPosition := false
		if addr != nil {
			if v := getEntry(5); v != nil {
				pos = &PositionState{}
				if err := Unmarshal(v, pos); err == nil {
					hasPosition = true
					result["your_position"] = map[string]interface{}{
						"shares_yes": pos.SharesYes,
						"shares_no":  pos.SharesNo,
						"cost_paid":  pos.CostPaid,
						"claimed":    pos.Claimed,
					}
				}
			} else {
				result["your_position"] = nil
			}
		}

		disputeBlocks := ComputeDisputeBlocks(mkt.OpenTime, mkt.ExpiryTime)
		if TEST_MODE {
			disputeBlocks = TEST_DISPUTE_BLOCKS
		}

		now := GetGlobalHeight()
		windowOpen := false
		var deadline uint64
		if hasProposal && !hasDispute && mkt.Status == STATUS_PROPOSED {
			deadline = proposal.ProposalBlock + disputeBlocks
			windowOpen = now > 0 && now < deadline
			result["dispute_window"] = map[string]interface{}{
				"open":           windowOpen,
				"proposal_block": proposal.ProposalBlock,
				"deadline_block": deadline,
				"window_blocks":  disputeBlocks,
				"current_height": now,
			}
		} else {
			result["dispute_window"] = map[string]interface{}{"open": false}
		}

		shouldDispute := false
		var reason string
		switch {
		case !hasProposal || hasDispute || mkt.Status != STATUS_PROPOSED:
			reason = "no active proposal to dispute"
		case now == 0:
			reason = "current height unavailable"
		case !windowOpen:
			reason = "dispute window closed or not yet open"
		case addr == nil:
			reason = "no address provided \u2014 cannot evaluate position"
		case !hasPosition || (pos.SharesYes == 0 && pos.SharesNo == 0):
			reason = "address holds no position in this market"
		case proposal.ProposedOutcome && pos.SharesYes > 0 && pos.SharesNo == 0:
			reason = "your position agrees with the proposed outcome"
		case !proposal.ProposedOutcome && pos.SharesNo > 0 && pos.SharesYes == 0:
			reason = "your position agrees with the proposed outcome"
		case proposal.ProposedOutcome && pos.SharesNo > 0:
			shouldDispute = true
			reason = "you hold NO shares but proposal resolves YES"
		case !proposal.ProposedOutcome && pos.SharesYes > 0:
			shouldDispute = true
			reason = "you hold YES shares but proposal resolves NO"
		default:
			reason = "mixed or ambiguous position \u2014 evaluate manually"
		}
		result["should_dispute"] = shouldDispute
		result["should_dispute_reason"] = reason

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	})

	log.Printf("plugin RPC server listening on %s (routes: /v1/query/markets, /v1/query/positions, /v1/query/resolvers, /v1/query/proposals, /v1/query/disputes, /v1/query/votes, /v1/query/outcomes, /v1/query/slashes, /v1/query/position, /v1/query/account, /v1/query/unbonding, /v1/query/dispute-context)", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("plugin RPC server error: %v", err)
	}
}
