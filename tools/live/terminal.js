'use strict';
(async()=>{
  const host=document.getElementById('terminal');
  const status=document.getElementById('state');
  const term=new Terminal({fontSize:16,scrollback:0,theme:{background:'#101318',foreground:'#e6edf3'}});
  const fit=new FitAddon.FitAddon();term.loadAddon(fit);term.open(host);
  window.liveTerminal=term;
  let socket,automatic=true,requested=16;
  const encoder=new TextEncoder();
  const send=(command,data)=>{if(socket?.readyState===WebSocket.OPEN)socket.send(encoder.encode(command+data));};
  function resize(){
    let font=requested;term.options.fontSize=font;
    if(automatic)for(let i=0;i<12;i++){
      const size=fit.proposeDimensions();if(!size||(size.cols>=44&&size.rows>=18)||font<=6)break;
      font=Math.max(6,Math.min(font-.5,Math.floor(font*Math.min(size.cols/44,size.rows/18)*2)/2));term.options.fontSize=font;
    }
    fit.fit();term.resize(Math.max(44,term.cols),Math.max(18,term.rows));
    send('1',JSON.stringify({columns:term.cols,rows:term.rows}));
  }
  new ResizeObserver(resize).observe(host);
  window.visualViewport?.addEventListener('resize',()=>{document.documentElement.style.setProperty('--vh',visualViewport.height+'px');resize();});
  document.getElementById('fit').onclick=()=>{automatic=true;requested=16;resize();};
  for(const[id,step]of[['out',-1],['in',1]])document.getElementById(id).onclick=()=>{automatic=false;requested=Math.max(6,Math.min(28,term.options.fontSize+step));resize();};
  document.getElementById('reload').onclick=()=>location.reload();
  term.onData(data=>send('0',data));
  for(const button of document.querySelectorAll('[data-key]'))button.onclick=()=>{send('0','\x1b'+button.dataset.key);if(button.dataset.key==='c')term.focus();else host.querySelector('textarea')?.blur();};
  const actions={tools:'\x1b[17~',up:'\x1b[A',down:'\x1b[B',enter:'\r',esc:'\x1b',stop:'\x1b[19~'};
  for(const button of document.querySelectorAll('[data-action]'))button.onclick=()=>send('0',actions[button.dataset.action]);
  resize();
  try{
    const response=await fetch('token');if(!response.ok)throw new Error('Не удалось авторизовать терминал');
    const auth=await response.json();const url=new URL('ws',location.href);url.protocol=location.protocol==='https:'?'wss:':'ws:';
    socket=new WebSocket(url,['tty']);socket.binaryType='arraybuffer';
    socket.onopen=()=>{socket.send(encoder.encode(JSON.stringify({AuthToken:auth.token,columns:term.cols,rows:term.rows})));status.textContent='Живой сервер · dsh и маркет · для ответов нужен настроенный провайдер LLM';window.liveReady=true;};
    socket.onmessage=event=>{const bytes=new Uint8Array(event.data);if(bytes[0]===48)term.write(bytes.slice(1));};
    socket.onclose=()=>{status.textContent='Соединение закрыто. Нажмите «Переподключить».';};
    socket.onerror=()=>{status.textContent='Ошибка подключения к серверу.';};
  }catch(error){status.textContent=error.message;}
})();
