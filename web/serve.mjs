import http from 'node:http'; import https from 'node:https'; import fs from 'node:fs'; import path from 'node:path';
const DIST=path.resolve('dist'); const UP='cramflow-production.up.railway.app';
const MIME={'.html':'text/html; charset=utf-8','.js':'text/javascript','.css':'text/css','.woff2':'font/woff2','.webp':'image/webp','.png':'image/png','.jpg':'image/jpeg'};
const stub=(b,p)=>{try{const d=JSON.parse(b);if(Array.isArray(d)){if(d[0])d[0].is_daily_pick=true;if(d[1])d[1].is_fresh=true;if(d[3])d[3].old_price=Math.round(d[3].price*1.3);if(d[4])d[4].stock=3;return JSON.stringify(d);}}catch{}return b;};
http.createServer((req,res)=>{const url=new URL(req.url,'http://x');
if(url.pathname.startsWith('/api')||url.pathname.startsWith('/uploads')){https.get({host:UP,path:req.url,headers:{host:UP}},up=>{if(!url.pathname.startsWith('/api/products')){res.writeHead(up.statusCode,up.headers);up.pipe(res);return;}let raw='';up.on('data',c=>raw+=c);up.on('end',()=>{res.writeHead(200,{'Content-Type':'application/json; charset=utf-8'});res.end(stub(raw,url.pathname));});}).on('error',()=>res.writeHead(502).end('{}'));return;}
let f=path.join(DIST,path.normalize(url.pathname));if(!f.startsWith(DIST)||!fs.existsSync(f)||fs.statSync(f).isDirectory())f=path.join(DIST,'index.html');
res.writeHead(200,{'Content-Type':MIME[path.extname(f)]||'application/octet-stream'});fs.createReadStream(f).pipe(res);}).listen(5204,()=>console.log('готов :5204'));
