// Live preview on the event form: the website card and the SMS reminder text,
// updated as the admin types. Pure enhancement — the panel stays hidden
// without JavaScript and nothing else depends on it.
(function () {
  var panel = document.getElementById("event-preview");
  var form = document.querySelector("form.form");
  if (!panel || !form) return;

  var org = panel.getAttribute("data-org") || "";
  var months = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
  var days = ["Sun","Mon","Tue","Wed","Thu","Fri","Sat"];

  function $(id) { return document.getElementById(id); }
  function field(name) {
    var el = form.querySelector('[name="' + name + '"]');
    return el ? el.value.trim() : "";
  }

  // Resolve the location text the same way the server will.
  function locationText() {
    var sel = $("locationChoice");
    if (!sel) return "";
    var v = sel.value;
    if (v === "none" || v === "") return "";
    if (v === "keep") return sel.getAttribute("data-current") || "";
    if (v === "new") {
      var name = field("newLocationName");
      var addr = field("newLocationAddress");
      if (!name) return addr;
      if (!addr || name.toLowerCase() === addr.toLowerCase()) return name;
      return name + " — " + addr;
    }
    var opt = sel.options[sel.selectedIndex];
    return (opt && opt.getAttribute("data-display")) || "";
  }

  function parseWhen() {
    var d = field("date"), t = field("start");
    if (!d) return null;
    var dp = d.split("-"), tp = (t || "00:00").split(":");
    if (dp.length !== 3) return null;
    var dt = new Date(+dp[0], +dp[1] - 1, +dp[2], +(tp[0] || 0), +(tp[1] || 0));
    return isNaN(dt.getTime()) ? null : dt;
  }

  function fmtTime(dt) {
    var h = dt.getHours(), m = dt.getMinutes();
    var ap = h >= 12 ? "PM" : "AM";
    h = h % 12; if (h === 0) h = 12;
    return h + ":" + (m < 10 ? "0" : "") + m + " " + ap;
  }

  function fmtEndTime() {
    var d = field("date"), e = field("end");
    if (!d || !e) return "";
    var dp = d.split("-"), tp = e.split(":");
    var dt = new Date(+dp[0], +dp[1] - 1, +dp[2], +(tp[0] || 0), +(tp[1] || 0));
    return isNaN(dt.getTime()) ? "" : fmtTime(dt);
  }

  function update() {
    var title = field("title") || "Event title";
    var dt = parseWhen();
    var loc = locationText();
    var isCommunity = false;
    var typeRadio = form.querySelector('[name="eventType"]:checked');
    if (typeRadio) isCommunity = typeRadio.value === "community";

    // --- website card ---
    $("pv-title").textContent = title;
    $("pv-type").hidden = !isCommunity;
    if (dt) {
      $("pv-month").textContent = months[dt.getMonth()];
      $("pv-day").textContent = dt.getDate();
      $("pv-year").textContent = dt.getFullYear();
    } else {
      $("pv-month").textContent = "—";
      $("pv-day").textContent = "–";
      $("pv-year").textContent = "";
    }
    var meta = [];
    if (dt) {
      var time = fmtTime(dt);
      var end = fmtEndTime();
      meta.push(end ? time + " – " + end : time);
    }
    if (loc) meta.push(loc);
    $("pv-meta").textContent = meta.join(" · ");
    $("pv-desc").textContent = field("description");
    var cn = field("contactName"), cp = field("contactPhone");
    $("pv-contact").textContent = cn || cp
      ? "Contact: " + [cn, cp].filter(Boolean).join(" · ")
      : "";

    // --- SMS reminder (post events only; mirrors the server's composition) ---
    var smsLabel = $("pv-sms-label"), sms = $("pv-sms"), note = $("pv-sms-note");
    if (isCommunity) {
      smsLabel.textContent = "Text reminder:";
      sms.hidden = true;
      note.textContent = "Community events are never sent as text reminders.";
      return;
    }
    sms.hidden = false;
    smsLabel.textContent = "Text reminder (sent from the reminder screen):";
    var body = org + ": Reminder — " + (field("title") || "…");
    if (dt) {
      body += " on " + days[dt.getDay()] + ", " + months[dt.getMonth()] + " " +
              dt.getDate() + " at " + fmtTime(dt);
    }
    if (loc) {
      var short = loc.split(" — ")[0];
      body += " at " + short;
    }
    body += ". Reply STOP to opt out.";
    sms.textContent = body;
    note.textContent = body.length + " characters" +
      (body.length > 160 ? " — may arrive as more than one message" : "");
  }

  form.addEventListener("input", update);
  form.addEventListener("change", update);
  panel.hidden = false;
  update();
})();
