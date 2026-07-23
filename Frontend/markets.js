const CLOSED_WINDOW = 20000; // blocks

function renderCurrentTab() {
  const el = document.getElementById('marketsList');
  if (!window._allMarkets.length) return;

  let markets;
  if (window._activeTab === 'live') {
    markets = window._allMarkets.filter(m => m.status === 0);
  } else if (window._activeTab === 'proposed') {
    markets = window._allMarkets.filter(m => m.status === 4 || m.status === 5);
  } else {
    // closed — rolling window of last CLOSED_WINDOW blocks
    markets = window._allMarkets.filter(m =>
      (m.status === 8 || m.status === 1 || m.status === 6 || m.status === 7 || m.status === 2 || m.status === 3) &&
      m.expiry && Number(m.expiry) >= (window.currentHeight - CLOSED_WINDOW)
    );
  }

  const countEl = document.getElementById('sb_c');
  if (countEl) countEl.textContent = window._allMarkets.filter(m => m.status === 0).length;

  if (markets.length === 0) {
    const labels = {live:'No open markets yet', proposed:'No markets awaiting resolution', closed:'No recently closed markets'};
    el.innerHTML = '<div class="alert ay">' + (labels[window._activeTab] || 'No markets') + '</div>';
    return;
  }
  let bookmarks = [];
  try { bookmarks = JSON.parse(localStorage.getItem('praxis_bookmarks') || '[]'); } catch {}
  el.innerHTML = '<div class="mgrid-2col">' + markets.map((m,i) => buildPraxisCard(m, bookmarks, i===0)).join('') + '</div>';

}
window.loadMarkets = async function () {
  const el = document.getElementById('marketsList');
  el.innerHTML = '<div class="loading"><span class="blink">▪ ▪ ▪</span>&nbsp;&nbsp;loading markets</div>';
  try {
    await checkRPC();

    const heightResp = await rpc('/v1/query/height', {});
    window.currentHeight = Number(heightResp.height || window.currentHeight || 1);

    const _pluginController = new AbortController();
    const _pluginTimeout = setTimeout(() => _pluginController.abort(), 10000);
    let pluginResp;
    try {
      pluginResp = await fetch(getPluginRPC() + '/v1/query/markets', { signal: _pluginController.signal });
    } catch (fetchErr) {
      if (fetchErr.name === 'AbortError') throw new Error('plugin RPC timed out after 10s');
      throw fetchErr;
    } finally {
      clearTimeout(_pluginTimeout);
    }
    if (!pluginResp.ok) throw new Error('plugin RPC returned ' + pluginResp.status);
    const raw = await pluginResp.json();

    const markets = (raw || []).map(entry => {
      const id = entry.id || '';
      const mk = entry.market || {};
      const qYes = BigInt(mk.q_yes || 0);
      const qNo  = BigInt(mk.q_no || 0);
      const expiry = BigInt(mk.expiry_time || 0);
      let status = (mk.status !== undefined && mk.status !== null) ? Number(mk.status) : 0;
      if (status === 0 && expiry && window.currentHeight > Number(expiry)) status = 8;
      // NOTE: resolver-driven states (proposed/finalized/disputed) are not
      // yet exposed by /v1/query/markets — only expiry-based live/closed
      // is derivable here. Extend the plugin response with a status field
      // to support the Proposed/Closed tabs fully.
      return {
        txHash: id,
        marketId: id,
        question: mk.question || '(no question)',
        rules: mk.rules || '',
        creator: mk.creator || '',
        b0: BigInt(mk.b_eff || 0),
        lmsrSeed: BigInt(mk.b_eff || 0),
        expiry,
        nonce: 0n,
        status,
        qYes,
        qNo,
      };
    });

    window._allMarkets = markets;
    window._allMarkets = markets;
    checkRoles();
    renderCurrentTab();
    return true;

  } catch (e) {
    el.innerHTML = '<div class="alert ar">⚠ Cannot reach plugin RPC at <code>' + getPluginRPC() + '</code><br>' + esc(e.message) + '</div>';
    return false;
  }
};
