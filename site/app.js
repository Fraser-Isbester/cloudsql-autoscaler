/* CloudSQL Autoscaler — hero console animation.
   A self-contained (no deps) canvas that streams a live CPU-utilization graph
   and periodically "resizes" the instance: as the machine type steps up/down,
   the graph renormalizes, a savings pill floats up, and a log line types in.
   The demand curve is a real function of time, so utilization is a physical
   consequence of demand / provisioned vCPUs — resizing visibly changes it. */
(function () {
  "use strict";

  var reduce =
    window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  /* ---- scroll reveal ---- */
  var revealables = document.querySelectorAll(".reveal");
  if ("IntersectionObserver" in window && !reduce) {
    var io = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (e) {
          if (e.isIntersecting) {
            e.target.classList.add("in");
            io.unobserve(e.target);
          }
        });
      },
      { threshold: 0.15 }
    );
    revealables.forEach(function (el) {
      io.observe(el);
    });
  } else {
    revealables.forEach(function (el) {
      el.classList.add("in");
    });
  }

  /* ---- console animation ---- */
  var canvas = document.getElementById("cpu");
  if (!canvas) return;
  var ctx = canvas.getContext("2d");
  var wrap = canvas.parentElement;

  var SIZES = [
    { name: "db-n2-standard-16", vcpu: 16 },
    { name: "db-n2-standard-8", vcpu: 8 },
    { name: "db-n2-standard-4", vcpu: 4 },
    { name: "db-n2-standard-2", vcpu: 2 },
  ];
  var PER_VCPU = 46; // ~$/vCPU/month, for the savings readout

  var idx = 1; // start on standard-8 (over-provisioned)
  var N = 150; // samples on screen
  var buf = [];
  var util = 34; // current utilization %
  var p95 = 34;
  var simMin = 9 * 60; // sim clock, minutes since midnight
  var lastSampleT = 0;
  var lastDecisionT = 0;
  var cooldownUntil = 0;
  var SAMPLE_MS = 60;
  var DECISION_MS = 2500;
  var COOLDOWN_MS = 2600;
  var markers = []; // {x:sampleAge, dir, age}

  var W = 0,
    H = 0,
    dpr = Math.min(window.devicePixelRatio || 1, 2);

  var mtypeEl = document.getElementById("mtype");
  var mnameEl = document.getElementById("mtype-name");
  var savingsEl = document.getElementById("savings");
  var logEl = document.getElementById("log");

  function resize() {
    W = wrap.clientWidth;
    H = wrap.clientHeight;
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  }
  if ("ResizeObserver" in window) new ResizeObserver(resize).observe(wrap);
  else window.addEventListener("resize", resize);
  resize();

  function demand(tSec) {
    // vCPU-equivalents of load, slowly wandering to create up & down pressure
    return 4.0 + 1.95 * Math.sin(tSec * 0.34) + 0.65 * Math.sin(tSec * 0.13 + 1);
  }
  function percentile(arr, p) {
    if (!arr.length) return 0;
    var s = arr.slice().sort(function (a, b) {
      return a - b;
    });
    return s[Math.min(s.length - 1, Math.floor(p * (s.length - 1)))];
  }
  function shortName(n) {
    return n.replace("db-n2-", "");
  }
  function clk(m) {
    var h = Math.floor(m / 60) % 24,
      mm = m % 60;
    return (h < 10 ? "0" + h : h) + ":" + (mm < 10 ? "0" + mm : mm);
  }

  var logs = [];
  function pushLog(html) {
    logs.unshift('<span class="ln">' + html + "</span>");
    logs = logs.slice(0, 2);
    logEl.innerHTML = logs.join("");
  }
  pushLog(
    '<span class="t">' +
      clk(simMin) +
      '</span> monitoring <span class="id">5 instances</span> · P95 over 3d'
  );

  function resizeInstance(dir) {
    var before = SIZES[idx];
    idx += dir > 0 ? 1 : -1;
    var after = SIZES[idx];
    simMin += 30;
    var diff = Math.abs(before.vcpu - after.vcpu) * PER_VCPU;

    if (mnameEl) mnameEl.textContent = after.name;
    if (mtypeEl) {
      mtypeEl.classList.remove("flip");
      void mtypeEl.offsetWidth;
      mtypeEl.classList.add("flip");
    }
    if (savingsEl) {
      savingsEl.textContent = (dir > 0 ? "−$" : "+$") + diff + "/mo";
      savingsEl.classList.toggle("up", dir < 0);
      savingsEl.classList.remove("show");
      void savingsEl.offsetWidth;
      savingsEl.classList.add("show");
    }
    markers.push({ age: 0, dir: dir });

    if (dir > 0) {
      pushLog(
        '<span class="t">' +
          clk(simMin) +
          '</span> <span class="ok">✓ scaled down</span> <span class="id">db-01</span> ' +
          shortName(before.name) +
          " → " +
          shortName(after.name) +
          ' · <span class="ok">-$' +
          diff +
          "/mo</span>"
      );
    } else {
      pushLog(
        '<span class="t">' +
          clk(simMin) +
          '</span> <span class="up">▲ scaled up</span> <span class="id">db-01</span> ' +
          shortName(before.name) +
          " → " +
          shortName(after.name) +
          ' · <span class="up">+$' +
          diff +
          "/mo</span>"
      );
    }
  }

  function color(v) {
    if (v < 45) return "#38bdf8"; // under-utilized
    if (v > 80) return "#fbbf24"; // hot
    return "#34d399"; // healthy
  }

  function draw(nowMs) {
    var padT = 16,
      padB = 14,
      padR = 2,
      padL = 2;
    var plotH = H - padT - padB;
    var y = function (v) {
      return padT + (1 - v / 100) * plotH;
    };
    var x = function (i) {
      return padL + (i / (N - 1)) * (W - padL - padR);
    };

    ctx.clearRect(0, 0, W, H);

    // target band 50–75%
    ctx.fillStyle = "rgba(52,211,153,0.05)";
    ctx.fillRect(0, y(75), W, y(50) - y(75));

    // gridlines
    ctx.strokeStyle = "rgba(255,255,255,0.05)";
    ctx.lineWidth = 1;
    [25, 50, 75].forEach(function (g) {
      ctx.beginPath();
      ctx.moveTo(0, y(g));
      ctx.lineTo(W, y(g));
      ctx.stroke();
    });

    // resize markers
    for (var m = markers.length - 1; m >= 0; m--) {
      var mk = markers[m];
      mk.age += 1;
      var mi = N - 1 - mk.age;
      if (mi < 0) {
        markers.splice(m, 1);
        continue;
      }
      var mx = x(mi);
      var alpha = Math.max(0, 1 - mk.age / N) * 0.5;
      ctx.strokeStyle =
        (mk.dir > 0 ? "rgba(52,211,153," : "rgba(251,191,36,") + alpha + ")";
      ctx.setLineDash([3, 4]);
      ctx.beginPath();
      ctx.moveTo(mx, padT);
      ctx.lineTo(mx, H - padB);
      ctx.stroke();
      ctx.setLineDash([]);
    }

    if (buf.length > 1) {
      var stroke = color(p95);
      var n = buf.length;
      var off = N - n; // right-align newest

      // area
      var grad = ctx.createLinearGradient(0, padT, 0, H - padB);
      grad.addColorStop(0, hexA(stroke, 0.28));
      grad.addColorStop(1, hexA(stroke, 0));
      ctx.beginPath();
      ctx.moveTo(x(off), y(buf[0]));
      for (var i = 1; i < n; i++) ctx.lineTo(x(off + i), y(buf[i]));
      ctx.lineTo(x(off + n - 1), H - padB);
      ctx.lineTo(x(off), H - padB);
      ctx.closePath();
      ctx.fillStyle = grad;
      ctx.fill();

      // line
      ctx.beginPath();
      ctx.moveTo(x(off), y(buf[0]));
      for (var j = 1; j < n; j++) ctx.lineTo(x(off + j), y(buf[j]));
      ctx.strokeStyle = stroke;
      ctx.lineWidth = 2;
      ctx.lineJoin = "round";
      ctx.stroke();

      // leading dot
      var lx = x(off + n - 1),
        ly = y(buf[n - 1]);
      ctx.fillStyle = stroke;
      ctx.beginPath();
      ctx.arc(lx, ly, 3.2, 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = hexA(stroke, 0.25);
      ctx.beginPath();
      ctx.arc(lx, ly, 6.5, 0, Math.PI * 2);
      ctx.fill();

      // P95 dashed line + label
      var py = y(p95);
      ctx.strokeStyle = "#a78bfa";
      ctx.setLineDash([5, 5]);
      ctx.lineWidth = 1.5;
      ctx.beginPath();
      ctx.moveTo(0, py);
      ctx.lineTo(W, py);
      ctx.stroke();
      ctx.setLineDash([]);
      var label = "P95 " + Math.round(p95) + "%";
      ctx.font =
        '600 11px "Geist Mono", ui-monospace, monospace';
      var tw = ctx.measureText(label).width + 12;
      ctx.fillStyle = "#8b5cf6";
      roundRect(ctx, W - tw - 6, py - 17, tw, 15, 4);
      ctx.fill();
      ctx.fillStyle = "#fff";
      ctx.textBaseline = "middle";
      ctx.fillText(label, W - tw, py - 9.5);
    }
  }

  function hexA(hex, a) {
    var r = parseInt(hex.slice(1, 3), 16),
      g = parseInt(hex.slice(3, 5), 16),
      b = parseInt(hex.slice(5, 7), 16);
    return "rgba(" + r + "," + g + "," + b + "," + a + ")";
  }
  function roundRect(c, x, y, w, h, r) {
    c.beginPath();
    c.moveTo(x + r, y);
    c.arcTo(x + w, y, x + w, y + h, r);
    c.arcTo(x + w, y + h, x, y + h, r);
    c.arcTo(x, y + h, x, y, r);
    c.arcTo(x, y, x + w, y, r);
    c.closePath();
  }

  function step(t) {
    if (!lastSampleT) lastSampleT = lastDecisionT = t;

    while (t - lastSampleT >= SAMPLE_MS) {
      lastSampleT += SAMPLE_MS;
      var tSec = lastSampleT / 1000;
      var cap = SIZES[idx].vcpu;
      var target = Math.max(3, Math.min(99, (demand(tSec) / cap) * 100));
      util += (target - util) * 0.18 + (Math.random() - 0.5) * 2.2;
      util = Math.max(2, Math.min(99, util));
      buf.push(util);
      if (buf.length > N) buf.shift();
      p95 += (percentile(buf, 0.95) - p95) * 0.1;
    }

    if (
      t - lastDecisionT >= DECISION_MS &&
      t >= cooldownUntil &&
      buf.length > 40
    ) {
      lastDecisionT = t;
      if (p95 < 46 && idx < SIZES.length - 1) {
        resizeInstance(1);
        cooldownUntil = t + COOLDOWN_MS;
      } else if (p95 > 82 && idx > 0) {
        resizeInstance(-1);
        cooldownUntil = t + COOLDOWN_MS;
      }
    }

    draw(t);
    requestAnimationFrame(step);
  }

  if (reduce) {
    // static representative frame
    for (var k = 0; k < N; k++) {
      var v = 60 + 12 * Math.sin(k * 0.15) + (Math.random() - 0.5) * 6;
      buf.push(Math.max(5, Math.min(95, v)));
    }
    p95 = percentile(buf, 0.95);
    if (mnameEl) mnameEl.textContent = SIZES[2].name;
    pushLog(
      '<span class="t">09:30</span> <span class="ok">✓ scaled down</span> <span class="id">db-01</span> standard-8 → standard-4 · <span class="ok">-$184/mo</span>'
    );
    draw(0);
  } else {
    requestAnimationFrame(step);
  }
})();
