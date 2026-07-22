// ═══════════════════════════════════════════
// RPC / NETWORK LAYER
// Extracted from app.js during modularization (2026-07-22).
// Still a plain classic script (not type="module") — loaded before
// app.js in index.html, sharing global scope exactly as before.
// See /mnt/user-data/uploads/praxis-modularization-plan.md for the
// full module boundary rationale.
// ═══════════════════════════════════════════

const getRPCHost = () => localStorage.getItem('praxis_rpc_host') || 'prax.val-a.grad.dev.app.canopynetwork.org';
const getRPC     = () => localStorage.getItem('praxis_rpc_host') ? `http://${getRPCHost()}:50002` : 'https://prax.val-a.grad.dev.app.canopynetwork.org/rpc';
const getPluginRPC = () => localStorage.getItem('praxis_plugin_rpc_host') ? `http://${localStorage.getItem('praxis_plugin_rpc_host')}` : 'https://prax.val-a.grad.dev.app.canopynetwork.org/plugin';
// Attach to window: top-level const/let does NOT auto-attach to window,
// unlike var/function declarations. index.html's separate inline <script>
// relies on window.getRPC for its footer connectivity poller — without this,
// that poller always fell back to a hardcoded, unreachable localhost:50002.
window.getRPCHost = getRPCHost;
window.getRPC = getRPC;
window.getPluginRPC = getPluginRPC;

async function rpc(path,body={}){
  const r=await fetch(getRPC()+path,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
  const t=await r.text();if(!r.ok)throw new Error(`HTTP ${r.status}: ${t}`);
  try{return JSON.parse(t);}catch{return t;}
}
async function submitTxRPC(obj){const d=await rpc('/v1/tx',obj);return typeof d==='string'?d.replace(/^"|"$/g,''):JSON.stringify(d);}

window.checkRPC=async function(){
  try{
    const d=await rpc('/v1/query/height',{});window.currentHeight=d.height||0;
    window.currentNetworkID=d.network_id||d.networkID||window.currentNetworkID;
    try{
      const blk=await rpc('/v1/query/block-by-height',{height:window.currentHeight});
      const hdr=blk?.blockHeader?.lastQuorumCertificate?.header;
      if(hdr){
        window.currentChainID=hdr.chainId||hdr.chainID||window.currentChainID;
        window.currentNetworkID=hdr.networkID||hdr.networkId||window.currentNetworkID;
      }
    }catch(e){console.warn('block-by-height chainId lookup failed',e);}
    ['rpcDot','rpcDotM'].forEach(id=>{const e=document.getElementById(id);if(e)e.className='dot live';});
    const el=document.getElementById('rpcStatus');if(el)el.textContent='live';
    const hb=document.getElementById('hBadge');if(hb)hb.textContent=`block ${window.currentHeight}`;
    const hm=document.getElementById('hbM');if(hm)hm.textContent=`#${window.currentHeight}`;
    ['ni_height'].forEach(id=>{const e=document.getElementById(id);if(e)e.textContent=window.currentHeight;});
    const ns=document.getElementById('ni_status');if(ns)ns.textContent='connected';
    const nr=document.getElementById('ni_rpc');if(nr)nr.textContent=getRPC();
    const sh=document.getElementById('sb_h');if(sh)sh.textContent=window.currentHeight;
    updateExpiryFromDate();
    const nonceEl=document.getElementById('c_nonce');if(nonceEl&&!nonceEl.value)nonceEl.value=BigInt(Date.now())*1000n;
    const ob=document.getElementById('offBanner');if(ob)ob.classList.remove('show');
    return true;
  }catch{
    ['rpcDot','rpcDotM'].forEach(id=>{const e=document.getElementById(id);if(e)e.className='dot';});
    const el=document.getElementById('rpcStatus');if(el)el.textContent='offline';
    const ns=document.getElementById('ni_status');if(ns)ns.textContent='offline';
    const ob=document.getElementById('offBanner');if(ob)ob.classList.add('show');
    return false;
  }
};
window.applyHost=function(){const h=document.getElementById('ni_host').value.trim();if(h)localStorage.setItem('praxis_rpc_host',h);checkRPC();toast('Connecting to '+h+'…');};

window.queryAccount=async function(){
  const addr=document.getElementById('w_addr').value.trim().toLowerCase();
  addr40(addr,'Address');
  try{
    const d=await rpc('/v1/query/account',{address:addr});
    document.getElementById('w_result').style.display='block';
    document.getElementById('w_balance').textContent=fmtPRX(d.amount||0);
    document.getElementById('w_addrD').textContent=addr;
  }catch(e){toast('Query failed: '+e.message,true);}
};

window.checkFailedTxs=async function(){
  const addr=document.getElementById('ft_addr').value.trim().toLowerCase();
  addr40(addr,'Address');
  try{
    const d=await rpc('/v1/query/failed-txs',{address:addr,perPage:20});
    const c=d.totalCount||0;const el=document.getElementById('ft_result');el.style.display='block';
    if(c===0){el.innerHTML=`<div class="alert ag">✓ No failed transactions for ${addr.slice(0,12)}…</div>`;return;}
    const rows=(d.results||[]).map(r=>`<div style="margin-bottom:8px;padding:8px;background:var(--bg);border:1px solid var(--border);font-family:'JetBrains Mono',monospace;font-size:10px"><span style="color:var(--red)">${esc(r.error?.msg||'?')} (${r.error?.code})</span><br><span style="color:var(--text3)">${r.txHash?.slice(0,24)}…</span></div>`).join('');
    el.innerHTML=`<div class="alert ar">⚠ ${c} failed tx(s)</div>${rows}`;
  }catch(e){toast('Query failed: '+e.message,true);}
};

