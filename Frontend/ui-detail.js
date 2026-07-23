window.showDetail = function(marketId) {
  const m = window._allMarkets.find(x => x.marketId === marketId || x.txHash === marketId);
  if (!m) return;
  const open = m.status === 0;
  const expired = m.status === 8;
  const cancelled = m.status === 1;
  const proposed = m.status === 4;
  const disputed = m.status === 5;
  const finalized = m.status === 6;
  const voided = m.status === 7;
  const resolved = m.status === 2;
  const total = m.qYes + m.qNo;
  const yesPct = total > 0n ? Number(m.qYes * 100n / total) : 50;
  const noPct = 100 - yesPct;
  const mid = m.marketId || m.txHash;

  document.getElementById('det-question').textContent = m.question;
  document.getElementById('det-qyes').textContent = fmtPRX(m.qYes) + ' PRX';
  document.getElementById('det-qno').textContent = fmtPRX(m.qNo) + ' PRX';
  document.getElementById('det-yes-pct').textContent = yesPct + '%';
  document.getElementById('det-no-pct').textContent = noPct + '%';
  const _outLbls = extractOutcomes(m.rules || '');
  const _yesLblEl = document.getElementById('det-yes-lbl');
  const _noLblEl = document.getElementById('det-no-lbl');
  if (_yesLblEl) _yesLblEl.textContent = _outLbls.yes;
  if (_noLblEl) _noLblEl.textContent = _outLbls.no;
  const _btnYesEl = document.getElementById('btn_yes');
  const _btnNoEl = document.getElementById('btn_no');
  if (_btnYesEl) _btnYesEl.textContent = _outLbls.yes;
  if (_btnNoEl) _btnNoEl.textContent = _outLbls.no;
  document.getElementById('det-bar').style.width = yesPct + '%';
  document.getElementById('det-mid').textContent = mid;
  document.getElementById('det-creator').textContent = m.creator || '—';
  document.getElementById('det-total').textContent = fmtPRX(m.qYes + m.qNo) + ' PRX';
  if (m.expiry) {
    const blk = Number(m.expiry);
    const blocksLeft = blk - window.currentHeight;
    const msLeft = blocksLeft * 5000;
    const expDate = new Date(Date.now() + msLeft);
    const dateStr = expDate.toLocaleDateString('en-US', {month:'short', day:'numeric', year:'numeric'});
    const timeStr = expDate.toLocaleTimeString('en-US', {hour:'2-digit', minute:'2-digit'});
    document.getElementById('det-expiry').textContent = 'blk #' + blk + '  (' + dateStr + ' ' + timeStr + ')';
  } else {
    document.getElementById('det-expiry').textContent = '—';
  }
  // Banner image
  const imgUrl = extractImg(m.rules || '');
  const bannerDiv = document.getElementById('det-img-banner');
  const bannerImg = document.getElementById('det-img-banner-img');
  if (bannerDiv && bannerImg) {
    if (imgUrl) {
      bannerImg.src = imgUrl;
      bannerDiv.style.display = '';
      bannerImg.onerror = () => { bannerDiv.style.display = 'none'; };
    } else {
      bannerDiv.style.display = 'none';
    }
  }

  const rulesRow = document.getElementById('det-rules-row');
  const rulesEl  = document.getElementById('det-rules');
  const catBadge = document.getElementById('det-cat-badge');
  if (rulesRow && rulesEl) {
    const rawRules = m.rules || '';
    const cat      = extractCat(rawRules);
    const stripped = stripImgTag(stripCatPrefix(rawRules)).trim();
    const displayRules = stripped || (cat !== 'other' ? 'No resolution criteria specified.' : '');
    if (displayRules || cat !== 'other') {
      rulesEl.textContent = displayRules || 'No resolution criteria specified.';
      rulesRow.style.display = '';
      if (catBadge) {
        catBadge.textContent = CAT_LABELS[cat] || '◈ Other';
        const catColors = { crypto:'#f7931a', sports:'#22c55e', politics:'#3b82f6', finance:'#a855f7', other:'var(--text3)' };
        catBadge.style.background = 'var(--surf2)';
        catBadge.style.color = catColors[cat] || 'var(--text3)';
        catBadge.style.border = '1px solid var(--border)';
      }
    } else {
      rulesRow.style.display = 'none';
    }
  }

  const resolverRow = document.getElementById('det-resolver-row');
  if (m.resolver) {
    resolverRow.style.display = '';
    const tier = m.resolver ? resolverTier(m.resolver) : null;
  const tierHtml = tier ? ' <span style="color:' + tier.color + '">' + tier.icon + ' ' + tier.label + '</span>' : '';
  document.getElementById('det-resolver').innerHTML = (m.resolver || '—') + tierHtml + (m.proposedOutcome !== undefined ? ' → proposed ' + (m.proposedOutcome ? '<span style="color:var(--green)">YES</span>' : '<span style="color:var(--red)">NO</span>') : '');
  } else {
    resolverRow.style.display = 'none';
  }

  const statusLabels = {0:'Open',1:'Cancelled',2:'Resolved',3:'Expired',4:'Proposed',5:'Disputed',6:'Finalized',7:'Voided',8:'Expired'};
  const statusClasses = {0:'sp-o',1:'sp-d',2:'sp-f',3:'sp-e',4:'sp-e',5:'sp-d',6:'sp-f',7:'sp-e',8:'sp-e'};
  document.getElementById('det-status-pill').innerHTML = '<div class="spill ' + (statusClasses[m.status]||'sp-f') + '"><span class="dot"></span>' + (statusLabels[m.status]||'Closed') + '</div>';


  const proposeBtn = document.getElementById('det-propose-btn');
  const claimBtn   = document.getElementById('det-claim-btn');
  if (proposeBtn) {
    if (m.status === 8) {
      // COI-1: hide propose if signer is market creator
      const signerIsCreator = signerAddress && m.creator && signerAddress.toLowerCase() === m.creator.toLowerCase();
      // COI-2: hide propose if signer holds a position in this market
      const signerHasPosition = (() => {
        try {
          const txs = JSON.parse(localStorage.getItem('praxis_tx_cache') || '[]');
          return txs.some(tx =>
            tx.messageType === 'submit_prediction' &&
            tx.sender && tx.sender.toLowerCase() === (signerAddress||'').toLowerCase() &&
            tx.transaction && tx.transaction.msg &&
            (() => { try { return b2h(Uint8Array.from(atob(tx.transaction.msg.marketId||''), c=>c.charCodeAt(0))) === mid; } catch { return false; } })()
          );
        } catch { return false; }
      })();
      if (signerIsCreator) {
        proposeBtn.style.display = '';
        proposeBtn.disabled = true;
        proposeBtn.title = 'Market creators cannot propose outcomes for their own markets';
        proposeBtn.textContent = '⚖ Cannot Propose (Creator)';
      } else if (signerHasPosition) {
        proposeBtn.style.display = '';
        proposeBtn.disabled = true;
        proposeBtn.title = 'Forfeit your position before proposing';
        proposeBtn.textContent = '⚖ Forfeit Position First';
      } else {
        proposeBtn.style.display = '';
        proposeBtn.disabled = false;
        proposeBtn.textContent = '⚖ Propose Outcome';
        proposeBtn.setAttribute('onclick', 'fillPropose(' + JSON.stringify(mid) + ')');
      }
    } else {
      proposeBtn.style.display = 'none';
      proposeBtn.disabled = false;
      proposeBtn.textContent = '⚖ Propose Outcome';
    }
  }
  if (claimBtn) {
    if (m.status === 6) {
      claimBtn.style.display = '';
      claimBtn.textContent = '◎ Claim Winnings';
      claimBtn.setAttribute('onclick', 'fillC(' + JSON.stringify(mid) + ')');
    } else if (m.status === 1) {
      claimBtn.style.display = '';
      claimBtn.textContent = '◎ Claim Refund';
      claimBtn.setAttribute('onclick', 'fillC(' + JSON.stringify(mid) + ')');
    }
  }

  const reclaimBtn = document.getElementById('det-reclaim-btn');
  if (reclaimBtn) {
    if (m.status === 8 && window.currentHeight > Number(m.expiry) + 300) {
      reclaimBtn.style.display = '';
      reclaimBtn.setAttribute('onclick', 'fillReclaim(' + JSON.stringify(mid) + ')');
    } else {
      reclaimBtn.style.display = 'none';
    }
  }

  const forfeitBtn = document.getElementById('det-forfeit-btn');
  if (forfeitBtn) {
    const signerHasPositionForForfeit = (() => {
      try {
        const txs = JSON.parse(localStorage.getItem('praxis_tx_cache') || '[]');
        return txs.some(tx =>
          tx.messageType === 'submit_prediction' &&
          tx.sender && tx.sender.toLowerCase() === (signerAddress||'').toLowerCase() &&
          tx.transaction && tx.transaction.msg &&
          (() => { try { return b2h(Uint8Array.from(atob(tx.transaction.msg.marketId||''), c=>c.charCodeAt(0))) === mid; } catch { return false; } })()
        );
      } catch { return false; }
    })();
    if (m.status === 0 && signerAddress && signerAddress !== m.creator && signerHasPositionForForfeit) {
      forfeitBtn.style.display = '';
      forfeitBtn.setAttribute('onclick', 'fillForfeit(' + JSON.stringify(mid) + ')');
    } else {
      forfeitBtn.style.display = 'none';
    }
  }

  const bannerCard = document.getElementById('det-banner-card');
  if (m.status === 8) {
    bannerCard.style.display = '';
    bannerCard.innerHTML = '<div class="mc-banner bnr"><span>⏳</span> Awaiting resolver proposal</div>';
  } else if (m.status === 4) {
    bannerCard.style.display = '';
    bannerCard.innerHTML = '<div class="mc-banner bnr"><span>🔎</span> Resolver: ' + (m.resolver ? m.resolver.slice(0,8) + '…' : '?') + ' — proposed ' + (m.proposedOutcome ? '<span style="color:var(--green)">YES</span>' : '<span style="color:var(--red)">NO</span>') + '</div>';
  } else if (m.status === 1) {
    bannerCard.style.display = '';
    bannerCard.innerHTML = '<div class="mc-banner bnr"><span>✕</span> Market cancelled — reclaim your stake</div>';
  } else if (m.status === 7) {
    bannerCard.style.display = '';
    bannerCard.innerHTML = '<div class="mc-banner bnr"><span>⚠</span> Market voided — full refund available</div>';
  } else {
    bannerCard.style.display = 'none';
  }

  const _pmidEl = document.getElementById('p_mid'); if (_pmidEl) _pmidEl.value = mid;
  showPage('detail', null, true);
  {
    const _path = '/detail/' + encodeURIComponent(mid);
    if(location.pathname !== _path) history.pushState({page:'detail', mid}, '', _path);
  }

  // Hero image
  if (m) {
    // Hero image
    const imgUrl = (typeof extractImg === 'function') ? extractImg(m.rules || '') : '';
    const heroWrap  = document.getElementById('det-hero-img-wrap');
    const heroBg    = document.getElementById('det-img-banner-img');
    const heroThumb = document.getElementById('det-img-thumb');
    const heroEmpty = document.getElementById('det-hero-empty');
    if (imgUrl) {
      if (heroBg)    { heroBg.src = imgUrl; heroBg.style.display = ''; }
      if (heroThumb) heroThumb.src = imgUrl;
      if (heroWrap)  heroWrap.style.display = '';
      if (heroEmpty) heroEmpty.style.display = 'none';
    } else {
      if (heroBg)    heroBg.style.display = 'none';
      if (heroWrap)  heroWrap.style.display = 'none';
      if (heroEmpty) heroEmpty.style.display = '';
    }

    // Category badge
    const catKey = (typeof extractCat === 'function') ? extractCat(m.rules || '') : 'other';
    const catIcons = {crypto:'🪙',sports:'⚽',politics:'🗳',finance:'📈',esports:'🎮',other:'◈'};
    const catBadgeHero = document.getElementById('det-cat-badge-hero');
    if (catBadgeHero) catBadgeHero.textContent = (catIcons[catKey] || '◈') + ' ' + catKey.charAt(0).toUpperCase() + catKey.slice(1);

    // Expiry (also fill raw for info tab)
    const rawEl = document.getElementById('det-expiry-raw');
    if (rawEl && m.expiry) rawEl.textContent = 'Block #' + m.expiry.toString();

    // YES/NO pct on outcome buttons
    const total = (m.qYes || 0n) + (m.qNo || 0n);
    const yesPct = total > 0n ? Number(m.qYes * 100n / total) : 50;
    const noPct  = 100 - yesPct;
    window._detailMarketId = mid;
  }

  // Render chart + holders sidebar
  setTimeout(() => {
    renderDetailChart(mid);
    renderHoldersSidebar(mid);
  }, 80);
  setTimeout(()=>switchDetailTab('activity'), 50);
};

window.renderActivityFeed = function(mid) {
  const el = document.getElementById('dpane-activity');
  if(!el||!mid) return;
  try {
    const txs = JSON.parse(localStorage.getItem('praxis_tx_cache')||'[]');
    const relevant = txs.filter(tx => {
      const msg = (tx.transaction&&tx.transaction.msg)||{};
      const rawMid = msg.marketId||msg.market_id||'';
      if(!rawMid && tx.messageType==='create_market') {
        // match by derived marketId
        return false; // handled below
      }
      if(!rawMid) return false;
      let txMid = rawMid;
      try { txMid = b2h(Uint8Array.from(atob(rawMid),c=>c.charCodeAt(0))); } catch{}
      return txMid === mid;
    });

    // also include the create_market TX for this market
    const createTx = txs.find(tx => tx.messageType==='create_market' && window._allMarkets.find(m=>m.marketId===mid&&m.txHash===tx.txHash));
    if(createTx && !relevant.includes(createTx)) relevant.unshift(createTx);

    relevant.sort((a,b)=>(b.height||0)-(a.height||0));

    if(!relevant.length){
      el.innerHTML='<div style="padding:20px;text-align:center;font-family:var(--font-mono);font-size:11px;color:var(--text3)">No activity found</div>';
      return;
    }

    const typeIcon  = {create_market:'◎',submit_prediction:'⚡',propose_outcome:'⚖',finalize_market:'✓',cancel_market:'✕',claim_winnings:'◈',forfeit_position:'↩',resolve_market:'⚑'};
    const typeColor = {create_market:'var(--text2)',submit_prediction:'var(--green)',propose_outcome:'#FFD700',finalize_market:'var(--green)',cancel_market:'var(--red)',claim_winnings:'var(--green)',forfeit_position:'var(--red)',resolve_market:'#C0C0C0'};

    el.innerHTML = relevant.map(tx => {
      const msg = (tx.transaction&&tx.transaction.msg)||{};
      const type = tx.messageType||'unknown';
      const icon = typeIcon[type]||'▪';
      const color = typeColor[type]||'var(--text3)';
      const sender = tx.sender||'';
      const height = tx.height||0;
      let detail = '';
      if(type==='submit_prediction'){
        const outcome = msg.outcome===true||msg.outcome==='true'||msg.outcome===1;
        const shares = BigInt(msg.shares||msg.amount||0);
        detail = '<span style="color:'+(outcome?'var(--green)':'var(--red)')+'font-weight:700">'+(outcome?'YES':'NO')+'</span> &nbsp;'+fmtPRX(shares)+' PRX';
      } else if(type==='propose_outcome'){
        const outcome = msg.proposedOutcome===true||msg.proposedOutcome==='true'||msg.proposedOutcome===1;
        detail = 'Proposed <span style="color:'+(outcome?'var(--green)':'var(--red)')+'">'+( outcome?'YES':'NO')+'</span>';
      } else if(type==='create_market'){
        const b0=BigInt(msg.b0||0);
        detail='Market created · B0 '+fmtPRX(b0)+' PRX';
      } else if(type==='finalize_market'){detail='Market finalized';}
      else if(type==='cancel_market'){detail='Market cancelled';}
      else if(type==='claim_winnings'){detail='Claimed winnings';}
      else if(type==='forfeit_position'){detail='Position forfeited';}
      return '<div style="display:flex;align-items:flex-start;gap:12px;padding:12px 16px;border-bottom:1px solid var(--border)">'+
        '<div style="font-size:15px;color:'+color+';min-width:18px;margin-top:1px">'+icon+'</div>'+
        '<div style="flex:1;min-width:0">'+
          '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:3px">'+
            '<span style="font-family:var(--font-mono);font-size:10px;color:'+color+';text-transform:uppercase;letter-spacing:.5px">'+type.replace(/_/g,' ')+'</span>'+
            '<span style="font-family:var(--font-mono);font-size:9px;color:var(--text3)">blk #'+height+'</span>'+
          '</div>'+
          '<div style="font-family:var(--font-mono);font-size:9px;color:var(--text3);margin-bottom:3px">'+
            (sender?sender.slice(0,8)+'…'+sender.slice(-6):'')+
          '</div>'+
          (detail?'<div style="font-family:var(--font-mono);font-size:11px;color:var(--text2)">'+detail+'</div>':'')+
        '</div>'+
      '</div>';
    }).join('');
  } catch(e) {
    el.innerHTML='<div style="padding:16px;color:var(--red);font-family:var(--font-mono);font-size:11px">Error: '+esc(e.message)+'</div>';
  }
};

window.renderTopHolders = async function(mid) {
  const el = document.getElementById('dpane-holders');
  if(!el||!mid) return;
  el.innerHTML='<div style="padding:20px;text-align:center;font-family:var(--font-mono);font-size:11px;color:var(--text3)">Loading holders…</div>';
  try {
    const resp = await fetch(getPluginRPC() + '/v1/query/positions?market=' + encodeURIComponent(mid));
    if(!resp.ok) throw new Error('positions query returned ' + resp.status);
    const raw = await resp.json();
    const list = (raw||[]).map(h => ({
      addr: h.address || '',
      yes: BigInt(h.sharesYes || 0),
      no: BigInt(h.sharesNo || 0),
      txCount: 1,
    }));
    if(!list.length){
      el.innerHTML='<div style="padding:20px;text-align:center;font-family:var(--font-mono);font-size:11px;color:var(--text3)">No positions yet</div>';
      return;
    }
    list.sort((a,b)=>Number((b.yes+b.no)-(a.yes+a.no)));
    const totalYes=list.reduce((s,h)=>s+h.yes,0n);
    const totalNo=list.reduce((s,h)=>s+h.no,0n);
    const grand=totalYes+totalNo;
    el.innerHTML=
      '<div style="display:grid;grid-template-columns:1fr 1fr;gap:1px;background:var(--border);border-bottom:1px solid var(--border)">'+
        '<div style="padding:12px;text-align:center;background:var(--surface)">'+
          '<div style="font-family:var(--font-mono);font-size:8px;color:var(--text3);margin-bottom:4px;letter-spacing:1px">TOTAL YES</div>'+
          '<div style="font-family:var(--font-mono);font-size:12px;color:var(--green)">'+fmtPRX(totalYes)+' PRX</div></div>'+
        '<div style="padding:12px;text-align:center;background:var(--surface)">'+
          '<div style="font-family:var(--font-mono);font-size:8px;color:var(--text3);margin-bottom:4px;letter-spacing:1px">TOTAL NO</div>'+
          '<div style="font-family:var(--font-mono);font-size:12px;color:var(--red)">'+fmtPRX(totalNo)+' PRX</div></div>'+
      '</div>'+
      list.map((h,i)=>{
        const total=h.yes+h.no;
        const pct=grand>0n?Number(total*100n/grand):0;
        const yesPct=total>0n?Number(h.yes*100n/total):0;
        const bias=h.yes>h.no?'YES':h.no>h.yes?'NO':'EVEN';
        const bc=bias==='YES'?'var(--green)':bias==='NO'?'var(--red)':'var(--text3)';
        return '<div style="padding:12px 16px;border-bottom:1px solid var(--border)">'+
          '<div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px">'+
            '<div style="display:flex;align-items:center;gap:8px">'+
              '<span style="font-family:var(--font-mono);font-size:9px;color:var(--text3);min-width:20px">#'+(i+1)+'</span>'+
              '<span style="font-family:var(--font-mono);font-size:10px;color:var(--green)">'+h.addr.slice(0,8)+'…'+h.addr.slice(-6)+'</span>'+
            '</div>'+
            '<span style="font-family:var(--font-mono);font-size:10px;font-weight:700;color:'+bc+'">'+bias+'</span>'+
          '</div>'+
          '<div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:6px;margin-bottom:8px">'+
            '<div><div style="font-family:var(--font-mono);font-size:8px;color:var(--text3);margin-bottom:2px">YES</div>'+
              '<div style="font-family:var(--font-mono);font-size:10px;color:var(--green)">'+fmtPRX(h.yes)+'</div></div>'+
            '<div><div style="font-family:var(--font-mono);font-size:8px;color:var(--text3);margin-bottom:2px">NO</div>'+
              '<div style="font-family:var(--font-mono);font-size:10px;color:var(--red)">'+fmtPRX(h.no)+'</div></div>'+
            '<div><div style="font-family:var(--font-mono);font-size:8px;color:var(--text3);margin-bottom:2px">SHARE</div>'+
              '<div style="font-family:var(--font-mono);font-size:10px;color:var(--text2)">'+pct+'%</div></div>'+
          '</div>'+
          '<div style="height:3px;background:var(--border);border-radius:2px;overflow:hidden">'+
            '<div style="height:100%;width:'+yesPct+'%;background:var(--green);transition:width .3s"></div>'+
          '</div>'+
        '</div>';
      }).join('');
  } catch(e){
    el.innerHTML='<div style="padding:16px;color:var(--red);font-family:var(--font-mono);font-size:11px">Error: '+esc(e.message)+'</div>';
  }
};

// ══════════════════════════════════════
// PREMIUM DETAIL PAGE JS
// ══════════════════════════════════════

// ── Patch switchDetailTab to handle new 'info' tab ──
window.switchDetailTab = function(tab) {
  ['activity','holders','info'].forEach(t => {
    const btn  = document.getElementById('dtab-' + t);
    const pane = document.getElementById('dpane-' + t);
    if (btn)  btn.classList.toggle('active', t === tab);
    if (pane) pane.style.display = t === tab ? '' : 'none';
  });
  if (tab === 'activity') renderActivityFeed && renderActivityFeed(window._detailMarketId);
  if (tab === 'holders')  renderTopHolders  && renderTopHolders(window._detailMarketId);
  if (tab === 'info')     renderDetailInfo  && renderDetailInfo(window._detailMarketId);
};

// ── Render det-info pane from current market ──
window.renderDetailInfo = function(mid) {
  if (!mid || !window._allMarkets) return;
  if (typeof renderDisputeCountdown === 'function') renderDisputeCountdown(mid);
  const m = window._allMarkets.find(x => x.marketId === mid || x.txHash === mid);
  if (!m) return;
  const raw = m.rules || '';
  const cat = (typeof extractCat === 'function') ? extractCat(raw) : 'other';
  const stripped = (typeof stripImgTag === 'function' && typeof stripCatPrefix === 'function')
    ? stripImgTag(stripCatPrefix(raw)).trim() : raw;
  const rulesRow = document.getElementById('det-rules-row');
  const rulesEl  = document.getElementById('det-rules');
  if (rulesRow && rulesEl && stripped) {
    rulesEl.textContent = stripped;
    rulesRow.style.display = '';
  } else if (rulesRow) {
    rulesRow.style.display = 'none';
  }
};
window._holderTab = 'yes';
window.setHolderTab = function(side, btn) {
  window._holderTab = side;
  document.querySelectorAll('.det-htab').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  renderHoldersSidebar(window._detailMarketId);
};

window.renderHoldersSidebar = function(mid) {
  const el = document.getElementById('det-holders-list');
  if (!el || !mid) return;
  try {
    const txs = JSON.parse(localStorage.getItem('praxis_tx_cache') || '[]');
    const holders = new Map();
    for (const tx of txs) {
      if (tx.messageType !== 'submit_prediction') continue;
      const msg = (tx.transaction && tx.transaction.msg) || {};
      const rawMid = msg.marketId || msg.market_id || '';
      let txMid = rawMid;
      try { txMid = b2h(Uint8Array.from(atob(rawMid), c=>c.charCodeAt(0))); } catch {}
      if (txMid !== mid) continue;
      const addr = tx.sender || '';
      const outcome = msg.outcome === true || msg.outcome === 'true' || msg.outcome === 1;
      const shares = BigInt(msg.shares || msg.amount || 0);
      if (!holders.has(addr)) holders.set(addr, { addr, yes: 0n, no: 0n });
      const h = holders.get(addr);
      if (outcome) h.yes += shares; else h.no += shares;
    }
    if (!holders.size) {
      el.innerHTML = '<div class="det-holders-empty">No positions yet</div>';
      return;
    }
    const side = window._holderTab === 'no' ? 'no' : 'yes';
    const list = [...holders.values()]
      .filter(h => h[side] > 0n)
      .sort((a,b) => Number(b[side] - a[side]))
      .slice(0, 10);
    if (!list.length) {
      el.innerHTML = '<div class="det-holders-empty">No ' + side.toUpperCase() + ' holders yet</div>';
      return;
    }
    el.innerHTML = list.map((h, i) => {
      const init = h.addr.slice(0,2).toUpperCase();
      const shortAddr = h.addr.slice(0,6) + '…' + h.addr.slice(-4);
      const amt = fmtPRX ? fmtPRX(h[side]) : (Number(h[side])/1e6).toFixed(2);
      return `<div class="det-holder-row">
        <span class="det-holder-rank">#${i+1}</span>
        <div class="det-holder-avatar">${init}</div>
        <span class="det-holder-addr">${shortAddr}</span>
        <span class="det-holder-amt ${side}-side">${amt}</span>
      </div>`;
    }).join('');
  } catch(e) {
    el.innerHTML = '<div class="det-holders-empty">Error loading</div>';
  }
};

// ── Chart (sparkline of YES probability from activity) ──
window._chartRange = '1W';
window.setChartRange = function(range, btn) {
  window._chartRange = range;
  document.querySelectorAll('.det-ctab').forEach(b => b.classList.remove('active'));
  if (btn) btn.classList.add('active');
  renderDetailChart(window._detailMarketId);
};

window.renderDetailChart = function(mid) {
  const canvas = document.getElementById('det-chart');
  const emptyEl = document.getElementById('det-chart-empty');
  if (!canvas || !mid) return;

  try {
    const txs = JSON.parse(localStorage.getItem('praxis_tx_cache') || '[]');
    const points = [];

    // Walk through all predict txs and compute rolling YES%
    let qYes = 0, qNo = 0;
    const sorted = txs
      .filter(tx => {
        if (tx.messageType !== 'submit_prediction') return false;
        const msg = (tx.transaction && tx.transaction.msg) || {};
        const rawMid = msg.marketId || msg.market_id || '';
        let txMid = rawMid;
        try { txMid = b2h(Uint8Array.from(atob(rawMid), c=>c.charCodeAt(0))); } catch {}
        return txMid === mid;
      })
      .sort((a,b) => (a.height||0) - (b.height||0));

    if (sorted.length < 2) {
      if (emptyEl) emptyEl.style.display = '';
      canvas.style.opacity = '0';
      return;
    }

    for (const tx of sorted) {
      const msg = (tx.transaction && tx.transaction.msg) || {};
      const outcome = msg.outcome === true || msg.outcome === 'true' || msg.outcome === 1;
      const shares = Number(BigInt(msg.shares || msg.amount || 0)) / 1e6;
      if (outcome) qYes += shares; else qNo += shares;
      const total = qYes + qNo;
      if (total > 0) {
        points.push({ h: tx.height || 0, pct: qYes / total * 100 });
      }
    }

    if (points.length < 2) {
      if (emptyEl) emptyEl.style.display = '';
      canvas.style.opacity = '0';
      return;
    }

    if (emptyEl) emptyEl.style.display = 'none';
    canvas.style.opacity = '1';

    // Draw on canvas
    const ctx = canvas.getContext('2d');
    const W = canvas.offsetWidth || 560;
    const H = canvas.offsetHeight || 140;
    canvas.width = W;
    canvas.height = H;
    ctx.clearRect(0, 0, W, H);

    const pad = { t: 10, b: 10, l: 10, r: 10 };
    const iW = W - pad.l - pad.r;
    const iH = H - pad.t - pad.b;

    const minH = points[0].h, maxH = points[points.length-1].h;
    const rangeH = Math.max(maxH - minH, 1);

    // Grid lines at 25%, 50%, 75%
    ctx.strokeStyle = 'rgba(255,255,255,.04)';
    ctx.lineWidth = 1;
    [25, 50, 75].forEach(p => {
      const y = pad.t + iH - (p / 100) * iH;
      ctx.beginPath(); ctx.moveTo(pad.l, y); ctx.lineTo(pad.l + iW, y); ctx.stroke();
    });

    // 50% dashed line
    ctx.strokeStyle = 'rgba(255,255,255,.12)';
    ctx.setLineDash([4,4]);
    const y50 = pad.t + iH * 0.5;
    ctx.beginPath(); ctx.moveTo(pad.l, y50); ctx.lineTo(pad.l+iW, y50); ctx.stroke();
    ctx.setLineDash([]);

    // Line + gradient fill
    ctx.beginPath();
    points.forEach((pt, i) => {
      const x = pad.l + ((pt.h - minH) / rangeH) * iW;
      const y = pad.t + iH - (pt.pct / 100) * iH;
      i === 0 ? ctx.moveTo(x, y) : ctx.lineTo(x, y);
    });
    ctx.strokeStyle = '#00e87a';
    ctx.lineWidth = 2;
    ctx.stroke();

    // Fill under line
    const lastPt = points[points.length-1];
    const lastX = pad.l + ((lastPt.h - minH) / rangeH) * iW;
    ctx.lineTo(lastX, pad.t + iH);
    ctx.lineTo(pad.l, pad.t + iH);
    ctx.closePath();
    const grad = ctx.createLinearGradient(0, pad.t, 0, pad.t + iH);
    grad.addColorStop(0, 'rgba(0,232,122,.25)');
    grad.addColorStop(1, 'rgba(0,232,122,0)');
    ctx.fillStyle = grad;
    ctx.fill();

    // Current value dot
    const lastY = pad.t + iH - (lastPt.pct / 100) * iH;
    ctx.beginPath();
    ctx.arc(lastX, lastY, 5, 0, Math.PI*2);
    ctx.fillStyle = '#00e87a';
    ctx.fill();
    ctx.strokeStyle = '#fff';
    ctx.lineWidth = 2;
    ctx.stroke();

  } catch(e) {
    if (emptyEl) emptyEl.style.display = '';
  }
};


// Also patch existing switchDetailTab calls in app.js

