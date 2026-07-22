// ═══════════════════════════════════════════
// SIGNING KEY LIFECYCLE
// Extracted from app.js during modularization (2026-07-22).
// Still a plain classic script (not type="module") — loaded before
// app.js and rpc.js in index.html, sharing global scope exactly as
// before. signerPrivKey/signerPubKey/signerAddress changed from
// `let` to `var` specifically so they stay visible across separate
// <script> tags — top-level `let`/`const` do NOT share across
// classic script tags the way `var` and function declarations do.
// See /mnt/user-data/uploads/praxis-modularization-plan.md.
// ═══════════════════════════════════════════

var signerPrivKey = null, signerPubKey = null, signerAddress = null; // var (not let) so this stays visible across separate <script> tags — required now that crypto-keystore.js is a separate classic script

window.loadKey=async function(){
  const hex=document.getElementById('sk_input').value.trim().toLowerCase();
  if(hex.length!==64)return toast('Private key must be exactly 64 hex chars',true);
  try{
    if(!bls12_381)throw new Error('BLS library not loaded');
    signerPrivKey=h2b(hex);
    signerPubKey=bls12_381.getPublicKey(signerPrivKey);
    const hb=await crypto.subtle.digest('SHA-256',signerPubKey);
    signerAddress=b2h(new Uint8Array(hb).slice(0,20));
    document.getElementById('keyStatus').className='kstat loaded';
    document.getElementById('keyStatus').textContent='✓ loaded — '+signerAddress.slice(0,16)+'…';
    document.getElementById('sk_derived').style.display='block';
    document.getElementById('sk_pub').textContent=b2h(signerPubKey);
    document.getElementById('sk_addr').textContent=signerAddress;
    ['c_creator','p_bettor','r_resolver','cl_addr','s_from','w_addr','ft_addr',
     'reg_addr','pr_resolver','dis_addr','cv_voter','rv_voter','tal_addr','fin_addr','sl_addr',
     'fo_resolver','rc_addr','ccf_addr','can_addr','unst_addr','cub_addr'].forEach(id=>{
      const el=document.getElementById(id);if(el&&!el.value)el.value=signerAddress;
    });
    const _ski=document.getElementById('sk_input');if(_ski)_ski.value='';
    refreshBalance();
    loadMyPredictions();
    toast('Key loaded — '+signerAddress);
    const badge=document.getElementById('sessBadge');
    if(badge)badge.classList.remove('hidden');
    injectKeyboardCopyBtns();
    setTimeout(wireCopyBtns, 100);
    return true;
  }catch(e){
    signerPrivKey=signerPubKey=signerAddress=null;
    toast('Key load failed: '+e.message,true);
    return false;
  }
};

window.clearKey=function(){
  localStorage.removeItem('praxis_keystore');
  signerPrivKey=signerPubKey=signerAddress=null;
  document.getElementById('keyStatus').className='kstat';
  document.getElementById('keyStatus').textContent='○ No key loaded';
  document.getElementById('sk_derived').style.display='none';
  const _ski=document.getElementById('sk_input');if(_ski)_ski.value='';
  ['c_creator','p_bettor','cl_addr','s_from','w_addr',
   'reg_addr','pr_resolver','di_addr','cv_addr','rv_addr','ta_addr','fin_addr','sl_addr',
   'fo_resolver','rc_addr','cf_addr','can_addr','un_addr','ub_addr',
   'rrw-addr','brw-addr','crw-addr','irw-addr','prw-addr'].forEach(id=>{
    const el=document.getElementById(id);if(el)el.value='';
  });
  syncWalletPill(null);
  toast('Key cleared');
  const badge=document.getElementById('sessBadge');
  if(badge)badge.classList.add('hidden');
};

window.createKeystore = async function() {
  const pw  = document.getElementById('ks_new_pw').value;
  const pw2 = document.getElementById('ks_new_pw2').value;
  if (!pw) return toast('Enter a password', true);
  if (pw !== pw2) return toast('Passwords do not match', true);
  if (pw.length < 8) return toast('Password must be at least 8 characters', true);

  try {
    // generate new BLS private key (valid scalar)
    const privBytes = bls12_381.utils.randomPrivateKey();
    const pubKey    = bls12_381.getPublicKey(privBytes);
    const hash      = await crypto.subtle.digest('SHA-256', pubKey);
    const address   = b2h(new Uint8Array(hash).slice(0, 20));

    const { salt, iv, encrypted } = await encryptKey(privBytes, pw);

    const keystore = {
      version: 1,
      kdf: 'argon2id',
      publicKey: b2h(pubKey),
      keyAddress: address,
      salt, iv, encrypted,
      argon2: { time: ARGON2_TIME, mem: ARGON2_MEM, threads: ARGON2_THREADS, keylen: ARGON2_KEYLEN },
    };

    // download
    const blob = new Blob([JSON.stringify(keystore, null, 2)], { type: 'application/json' });
    const url  = URL.createObjectURL(blob);
    const a    = document.createElement('a');
    a.href = url; a.download = 'praxis-keystore-' + address.slice(0,8) + '.json';
    a.click(); URL.revokeObjectURL(url);

    // auto-load into session
    signerPrivKey = privBytes;
    signerPubKey  = pubKey;
    signerAddress = address;
    updateSignerUI();
    toast('Keystore created and loaded');
    document.getElementById('ks_new_pw').value = '';
    document.getElementById('ks_new_pw2').value = '';
  } catch(e) { toast('Create failed: ' + e.message, true); }
};

window.checkSavedKeystore = function() {
  const saved = localStorage.getItem('praxis_keystore');
  const wrap = document.getElementById('ks_quick_wrap');
  if (!wrap) return;
  if (saved) {
    const raw = JSON.parse(saved);
    const addr = raw.keyAddress || '?';
    document.getElementById('ks_quick_addr').textContent = addr.slice(0,8) + '…' + addr.slice(-6);
    wrap.style.display = '';
  } else {
    wrap.style.display = 'none';
  }
};

window.quickUnlock = async function() {
  const pw = document.getElementById('ks_quick_pw').value;
  if (!pw) return toast('Enter password', true);
  const saved = localStorage.getItem('praxis_keystore');
  if (!saved) return toast('No saved keystore', true);
  try {
    const raw = JSON.parse(saved);
    if (!raw.encrypted || !raw.salt || !raw.iv || !raw.publicKey) throw new Error('Invalid saved keystore');
    if (raw.argon2) { window._argon2Override = raw.argon2; } else { window._argon2Override = null; }
    let privBytes;
    try {
      privBytes = await decryptKey(raw.encrypted, raw.iv, raw.salt, pw, raw.kdf || 'argon2id');
    } catch(e) {
      privBytes = await decryptKey(raw.encrypted, raw.iv, raw.salt, pw, 'pbkdf2');
    }
    let pubKey = bls12_381.getPublicKey(privBytes);
    if (b2h(pubKey) !== raw.publicKey) {
      try { privBytes = await decryptKey(raw.encrypted, raw.iv, raw.salt, pw, 'pbkdf2'); pubKey = bls12_381.getPublicKey(privBytes); } catch(e2) {}
    }
    if (b2h(pubKey) !== raw.publicKey) throw new Error('Wrong password');
    const hash = await crypto.subtle.digest('SHA-256', pubKey);
    const address = b2h(new Uint8Array(hash).slice(0, 20));
    signerPrivKey = privBytes;
    signerPubKey = pubKey;
    signerAddress = address;
    updateSignerUI();
    toast('Session restored — ' + address.slice(0,8) + '…');
    document.getElementById('ks_quick_pw').value = '';
  } catch(e) { toast('Unlock failed: ' + e.message, true); }
};

window.importKeystore = async function() {
  const pw   = document.getElementById('ks_imp_pw').value;
  const file = document.getElementById('ks_imp_file').files[0];
  if (!file) return toast('Select a keystore file', true);
  if (!pw)   return toast('Enter password', true);

  try {
    const text = await file.text();
    const raw  = JSON.parse(text);

    // Praxis flat format
    if (!raw.encrypted || !raw.salt || !raw.iv || !raw.publicKey) throw new Error('Invalid keystore file');
    if (raw.argon2) { window._argon2Override = raw.argon2; } else { window._argon2Override = null; }
    let privBytes;
    try {
      privBytes = await decryptKey(raw.encrypted, raw.iv, raw.salt, pw, raw.kdf || 'argon2id');
    } catch(e) {
      privBytes = await decryptKey(raw.encrypted, raw.iv, raw.salt, pw, 'pbkdf2');
    }
    let pubKey = bls12_381.getPublicKey(privBytes);
    if (b2h(pubKey) !== raw.publicKey) {
      // try pbkdf2 fallback
      try {
        privBytes = await decryptKey(raw.encrypted, raw.iv, raw.salt, pw, 'pbkdf2');
        pubKey = bls12_381.getPublicKey(privBytes);
      } catch(e2) {}
    }
    if (b2h(pubKey) !== raw.publicKey) throw new Error('Wrong password or corrupted keystore');
    const hash    = await crypto.subtle.digest('SHA-256', pubKey);
    const address = b2h(new Uint8Array(hash).slice(0, 20));
    signerPrivKey = privBytes;
    signerPubKey  = pubKey;
    signerAddress = address;
    updateSignerUI();
    toast('Keystore unlocked — ' + address.slice(0,8) + '…');
    localStorage.setItem('praxis_keystore', JSON.stringify(raw));
    document.getElementById('ks_imp_pw').value = '';
    document.getElementById('ks_imp_file').value = '';
    checkSavedKeystore();
  } catch(e) { console.error('Import failed full error:', e); toast('Import failed: ' + e.message, true); }
};

function updateSignerUI() {
  document.getElementById('keyStatus').className = 'kstat loaded';
  document.getElementById('keyStatus').textContent = '✓ loaded — ' + signerAddress.slice(0,16) + '…';
  document.getElementById('sk_derived').style.display = 'block';
  document.getElementById('sk_pub').textContent = b2h(signerPubKey);
  document.getElementById('sk_addr').textContent = signerAddress;
  ['c_creator','p_bettor','cl_addr','s_from','w_addr',
   'reg_addr','pr_resolver','di_addr','cv_addr','rv_addr','ta_addr','fin_addr','sl_addr',
   'fo_resolver','rc_addr','cf_addr','can_addr','un_addr','ub_addr',
   'rrw-addr','brw-addr','crw-addr','irw-addr','prw-addr'].forEach(id => {
    const el = document.getElementById(id); if (el) el.value = signerAddress;
  });
  const badge = document.getElementById('sessBadge');
  if (badge) badge.classList.remove('hidden');
  syncWalletPill(signerAddress);
  refreshBalance();
  loadMyPredictions();
  injectKeyboardCopyBtns();
  setTimeout(wireCopyBtns, 100);
  checkRoles();
}

function syncWalletPill(address) {
  const short = address ? address.slice(0,8) + '…' + address.slice(-6) : 'Not connected';
  [['walletPill','wpAddr','wpDot','wpX'], ['walletPillM','wpAddrM','wpDotM',null]].forEach(([pillId, addrId, dotId, xId]) => {
    const pill = document.getElementById(pillId);
    const addrEl = document.getElementById(addrId);
    const dotEl = document.getElementById(dotId);
    if (addrEl) addrEl.textContent = short;
    if (pill) {
      pill.classList.toggle('connected', !!address);
      pill.classList.toggle('disconnected', !address);
    }
    if (xId) { const xEl = document.getElementById(xId); if (xEl) xEl.style.display = address ? '' : 'none'; }
  });
}

window.handleWalletPillClick = function() {
  if (signerAddress) {
    if (confirm('Disconnect wallet ' + signerAddress.slice(0,10) + '…?')) clearKey();
  } else {
    showPage('profile', document.querySelector('[data-p="profile"]'));
  }
};

