// Contact autofill on the event form: picking (or typing) a member's name
// fills their phone number. Free text always works — the datalist is only a
// suggestion source, so non-members are simply typed in as-is.
//
// The phone is only overwritten when it's empty or still holding a value we
// autofilled ourselves — a hand-edited number is never clobbered.
(function () {
  var nameInput = document.getElementById("contactName");
  var phoneInput = document.getElementById("contactPhone");
  var list = document.getElementById("member-names");
  var note = document.getElementById("contact-phone-note");
  if (!nameInput || !phoneInput || !list) return;

  var lastAutofill = "";

  // E.164 US (+17065551234) → (706) 555-1234 for friendly display.
  function pretty(phone) {
    var m = /^\+1(\d{3})(\d{3})(\d{4})$/.exec(phone);
    return m ? "(" + m[1] + ") " + m[2] + "-" + m[3] : phone;
  }

  function memberPhone(name) {
    var target = name.trim().toLowerCase();
    if (!target) return "";
    var opts = list.options;
    for (var i = 0; i < opts.length; i++) {
      if (opts[i].value.trim().toLowerCase() === target) {
        return opts[i].getAttribute("data-phone") || "";
      }
    }
    return "";
  }

  function sync() {
    var phone = memberPhone(nameInput.value);
    var current = phoneInput.value.trim();
    if (phone) {
      var display = pretty(phone);
      if (current === "" || current === lastAutofill) {
        phoneInput.value = display;
        lastAutofill = display;
        if (note) note.textContent = "Filled from the member list — edit if needed.";
        // Let the live preview pick up the programmatic change.
        phoneInput.dispatchEvent(new Event("input", { bubbles: true }));
      }
    } else if (current !== "" && current === lastAutofill) {
      // Name no longer matches a member; clear only what we autofilled.
      phoneInput.value = "";
      lastAutofill = "";
      if (note) note.textContent = "";
      phoneInput.dispatchEvent(new Event("input", { bubbles: true }));
    } else if (note && current === "") {
      note.textContent = "";
    }
  }

  nameInput.addEventListener("input", sync);
  nameInput.addEventListener("change", sync);
  phoneInput.addEventListener("input", function () {
    // Manual edits detach the autofill link.
    if (phoneInput.value.trim() !== lastAutofill) lastAutofill = "";
    if (note) note.textContent = "";
  });
})();
