// LiveTX web UI client. Talks to the Go server over a WebSocket: sends
// start/stop commands and renders the live transcript stream.
const $ = (id) => document.getElementById(id);
let ws, running = false, partialEl = null, autoScroll = true;

function setStatus(msg, isErr) {
  const s = $("status");
  s.textContent = msg;
  // Badge variant classes: secondary (idle) vs destructive (error).
  s.setAttribute("data-variant", isErr ? "destructive" : "secondary");
}

function setRunning(on) {
  running = on;
  $("toggle").textContent = on ? "Stop" : "Start";
  $("toggle").setAttribute("data-variant", on ? "destructive" : "default");
  $("dot").classList.toggle("bg-green-500", on);
  $("dot").classList.toggle("bg-muted-foreground", !on);
  $("device").disabled = on;
}

function connect() {
  const proto = location.protocol === "https:" ? "wss" : "ws";
  ws = new WebSocket(proto + "://" + location.host + "/ws");
  ws.onmessage = (ev) => {
    const m = JSON.parse(ev.data);
    if (m.type === "transcript") addTranscript(m);
    else if (m.type === "status") setStatus(m.text);
    else if (m.type === "error") { setStatus(m.text, true); setRunning(false); }
  };
  ws.onclose = () => { setStatus("Disconnected.", true); setRunning(false); };
}

// styleLine colors a transcript line by kind: source text stays white/default,
// translated text is yellow and a tad larger so it stands out.
function styleLine(el, isTranslation) {
  if (isTranslation) {
    el.style.color = "#facc15"; // yellow-400
    el.style.fontSize = "1.15em";
  } else {
    el.style.color = "";
    el.style.fontSize = "";
  }
}

function addTranscript(m) {
  const box = $("transcript");
  const placeholder = box.querySelector("p.text-center");
  if (placeholder) placeholder.remove();
  const text = (m.text || "").trim();
  if (!text) return;
  if (m.final) {
    if (partialEl) { partialEl.remove(); partialEl = null; }
    const p = document.createElement("p");
    p.className = "text-foreground";
    styleLine(p, m.translation);
    p.textContent = text;
    box.appendChild(p);
  } else {
    if (!partialEl) {
      partialEl = document.createElement("p");
      partialEl.className = "italic text-muted-foreground";
      box.appendChild(partialEl);
    }
    styleLine(partialEl, m.translation);
    partialEl.textContent = text;
  }
  if (autoScroll) box.parentElement.scrollTop = box.parentElement.scrollHeight;
}

// setAutoScroll toggles whether new transcript lines scroll the view to the
// bottom, reflecting the state in the footer button.
function setAutoScroll(on) {
  autoScroll = on;
  const b = $("autoscroll");
  b.textContent = "Auto-scroll: " + (on ? "On" : "Off");
  b.style.opacity = on ? "" : "0.55";
  if (on) {
    const box = $("transcript");
    box.parentElement.scrollTop = box.parentElement.scrollHeight;
  }
}
$("autoscroll").onclick = () => setAutoScroll(!autoScroll);

$("toggle").onclick = () => {
  if (!ws || ws.readyState !== WebSocket.OPEN) { setStatus("Not connected.", true); return; }
  if (running) {
    ws.send(JSON.stringify({ action: "stop" }));
    setRunning(false);
  } else {
    ws.send(JSON.stringify({
      action: "start",
      device: $("device").value,
      langIn: ($("langIn").value || "auto").trim(),
      langOut: ($("langOut").value || "en").trim(),
    }));
    setRunning(true);
  }
};

connect();
