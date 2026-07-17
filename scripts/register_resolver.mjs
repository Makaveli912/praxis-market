import { bls12_381 } from '@noble/curves/bls12-381';

const RPC = 'http://localhost:50002';
const PRIV_HEX = process.env.PRAXIS_SIGNER_KEY;
if (!PRIV_HEX) { console.error('Set PRAXIS_SIGNER_KEY env var first'); process.exit(1); }

const RESOLVER_ADDR = 'e7c7dad131a03f7ea0cc09a637ad096eb3495f77';
const STAKE = 500000000000n;
const FEE = 10000;
const NETWORK_ID = 1;
const CHAIN_ID = 1;

function h2b(hex){hex=hex.trim().toLowerCase();if(hex.length%2)throw new Error('Odd hex');const o=new Uint8Array(hex.length/2);for(let i=0;i<o.length;i++)o[i]=parseInt(hex.slice(i*2,i*2+2),16);return o;}
function b2h(b){return Array.from(b).map(x=>x.toString(16).padStart(2,'0')).join('');}
function cat(...a){const t=a.reduce((s,x)=>s+x.length,0);const o=new Uint8Array(t);let off=0;for(const x of a){o.set(x,off);off+=x.length;}return o;}
function encV(x){x=typeof x==='bigint'?x:BigInt(x);if(x===0n)return new Uint8Array([0]);const out=[];while(x>0n){let byte=Number(x&0x7fn);x>>=7n;if(x>0n)byte|=0x80;out.push(byte);}return new Uint8Array(out);}
function tag(f,w){return encV((BigInt(f)<<3n)|BigInt(w));}
function vf(f,v){const x=typeof v==='bigint'?v:BigInt(v);if(x===0n)return new Uint8Array(0);return cat(tag(f,0),encV(x));}
function bf(f,b){if(!b||!b.length)return new Uint8Array(0);return cat(tag(f,2),encV(b.length),b);}
function sf(f,s){if(!s||!s.length)return new Uint8Array(0);const e=new TextEncoder().encode(s);return cat(tag(f,2),encV(e.length),e);}
function ef(f,m){if(!m||!m.length)return new Uint8Array(0);return cat(tag(f,2),encV(m.length),m);}

function encRegister(addr,stake){return cat(bf(1,h2b(addr)),vf(2,stake));}
function encAny(typeUrl,inner){return cat(sf(1,typeUrl),bf(2,inner));}
function encSignBytes(msgType,typeUrl,inner,{txTime,fee,height,memo,netId,chainId}){
  const any=encAny(typeUrl,inner);
  return cat(sf(1,msgType),ef(2,any),vf(4,height),vf(5,txTime),vf(6,fee||10000),memo?sf(7,memo):new Uint8Array(0),vf(8,netId||1),vf(9,chainId||1));
}

async function main(){
  const privBytes = h2b(PRIV_HEX);
  const pubBytes = bls12_381.getPublicKey(privBytes);

  const heightResp = await fetch(RPC+'/v1/query/height',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({})});
  const heightData = await heightResp.json();
  const height = heightData.height || 1;
  console.log('Current height:', height);

  const txTime = BigInt(Date.now())*1000n;
  const inner = encRegister(RESOLVER_ADDR, STAKE);
  const typeUrl = 'type.googleapis.com/types.MessageRegisterResolver';
  const msgType = 'register_resolver';

  const signBytes = encSignBytes(msgType, typeUrl, inner, {txTime, fee: FEE, height, memo:'', netId: NETWORK_ID, chainId: CHAIN_ID});
  const sig = await bls12_381.sign(signBytes, privBytes);

  const tx = {
    signature: { publicKey: b2h(pubBytes), signature: b2h(sig) },
    createdHeight: height,
    time: Number(txTime),
    fee: FEE,
    memo: '',
    networkID: NETWORK_ID,
    chainID: CHAIN_ID,
    type: msgType,
    msgTypeUrl: typeUrl,
    msgBytes: b2h(inner),
  };

  console.log('Submitting tx...');
  const submitResp = await fetch(RPC+'/v1/tx',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(tx)});
  const text = await submitResp.text();
  console.log('HTTP', submitResp.status);
  console.log(text);
}

main().catch(e=>{console.error('ERROR:', e); process.exit(1);});
