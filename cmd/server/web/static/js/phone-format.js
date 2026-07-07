// Live phone formatting for every input[type="tel"] on any page.
//
// US numbers format progressively as you type: 7065550123 → (706) 555-0123,
// and 11 digits with a leading 1 → +1 (706) 555-0123. Values the user starts
// with "+" (other international numbers) are left alone. The server strips
// punctuation anyway (NormalizePhone), so formatting is purely for humans.
(function () {
  function formatUS(digits) {
    if (digits.length <= 3) return digits;
    if (digits.length <= 6) return "(" + digits.slice(0, 3) + ") " + digits.slice(3);
    return "(" + digits.slice(0, 3) + ") " + digits.slice(3, 6) + "-" + digits.slice(6, 10);
  }

  function format(value) {
    var v = value.trim();
    if (v === "") return v;
    var digits = v.replace(/\D/g, "");
    if (v[0] === "+") {
      // International-style entry: only normalize a COMPLETE +1 US number;
      // partial or non-US input is left exactly as typed.
      if (digits.length === 11 && digits[0] === "1") {
        return "+1 " + formatUS(digits.slice(1));
      }
      return v;
    }
    if (digits.length === 11 && digits[0] === "1") {
      return "+1 " + formatUS(digits.slice(1));
    }
    if (digits.length > 11) return v; // too long to be US — leave as typed
    return formatUS(digits);
  }

  function attach(input) {
    input.addEventListener("input", function (ev) {
      // Don't fight deletions or mid-string edits — reformat only when the
      // caret sits at the end and the user is adding characters. Everything
      // gets a final cleanup on blur.
      if (ev.inputType && ev.inputType.indexOf("delete") === 0) return;
      if (input.selectionStart !== input.value.length) return;
      var next = format(input.value);
      if (next !== input.value) input.value = next;
    });
    input.addEventListener("blur", function () {
      var next = format(input.value);
      if (next !== input.value) input.value = next;
    });
  }

  document.querySelectorAll('input[type="tel"]').forEach(attach);
})();
