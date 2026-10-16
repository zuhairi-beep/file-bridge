package main

import (
	"net/http"
)

// SHA-256 helper for the mobile upload page (loaded on demand to keep the HTML small for iOS Safari).
const uploadSha256JS = `var K=new Uint32Array([0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2]);
function Sha256(){this.h=new Uint32Array([0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19]);this.buf=new Uint8Array(64);this.bufLen=0;this.len=0;this.w=new Uint32Array(64);}
Sha256.prototype.block=function(d,o){var w=this.w,h=this.h,i,t1,t2;for(i=0;i<16;i++)w[i]=(d[o+4*i]<<24)|(d[o+4*i+1]<<16)|(d[o+4*i+2]<<8)|d[o+4*i+3];for(i=16;i<64;i++){var x=w[i-15],y=w[i-2];w[i]=(((x>>>7)|(x<<25))^((x>>>18)|(x<<14))^(x>>>3))+w[i-16]+(((y>>>17)|(y<<15))^((y>>>19)|(y<<13))^(y>>>10))+w[i-7];}var a=h[0],b=h[1],c=h[2],e=h[4],f=h[5],g=h[6],hh=h[7],dd=h[3];for(i=0;i<64;i++){t1=(hh+(((e>>>6)|(e<<26))^((e>>>11)|(e<<21))^((e>>>25)|(e<<7)))+((e&f)^(~e&g))+K[i]+w[i])|0;t2=((((a>>>2)|(a<<30))^((a>>>13)|(a<<19))^((a>>>22)|(a<<10)))+((a&b)^(a&c)^(b&c)))|0;hh=g;g=f;f=e;e=(dd+t1)|0;dd=c;c=b;b=a;a=(t1+t2)|0;}h[0]+=a;h[1]+=b;h[2]+=c;h[3]+=dd;h[4]+=e;h[5]+=f;h[6]+=g;h[7]+=hh;};
Sha256.prototype.update=function(d){var i=0,n=d.length;this.len+=n;if(this.bufLen){while(i<n&&this.bufLen<64)this.buf[this.bufLen++]=d[i++];if(this.bufLen<64)return;this.block(this.buf,0);this.bufLen=0;}for(;i+64<=n;i+=64)this.block(d,i);while(i<n)this.buf[this.bufLen++]=d[i++];};
Sha256.prototype.hex=function(){var bits=this.len*8,pad=new Uint8Array(((this.bufLen<56)?56:120)-this.bufLen+8);pad[0]=0x80;var hi=Math.floor(bits/4294967296),lo=bits>>>0,p=pad.length;pad[p-8]=hi>>>24;pad[p-7]=hi>>>16;pad[p-6]=hi>>>8;pad[p-5]=hi;pad[p-4]=lo>>>24;pad[p-3]=lo>>>16;pad[p-2]=lo>>>8;pad[p-1]=lo;var len=this.len;this.update(pad);this.len=len;var s='';for(var i=0;i<8;i++)s+=('00000000'+this.h[i].toString(16)).slice(-8);return s;};
function sha256File(f){var CHUNK=4<<20,h=new Sha256(),off=0;function step(){if(off>=f.size)return Promise.resolve(h.hex());var slice=f.slice(off,off+CHUNK);return new Promise(function(res,rej){var fr=new FileReader();fr.onload=function(){h.update(new Uint8Array(fr.result));off+=CHUNK;res();};fr.onerror=function(){rej(fr.error);};fr.readAsArrayBuffer(slice);}).then(step);}return step();}`

func (fs *FileServer) handleUploadSha256JS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write([]byte(uploadSha256JS))
}
