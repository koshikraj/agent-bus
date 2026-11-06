/* @ds-bundle: {"format":4,"namespace":"Birdie","components":[{"name":"Logo"},{"name":"Birds"},{"name":"Letter"},{"name":"Button"},{"name":"InstallCommand"},{"name":"TextField"},{"name":"AgentAvatar"},{"name":"AgentRow"},{"name":"MessageBubble"},{"name":"StatusSignal"}]} */
(function () {
  var React = window.React;
  var h = React.createElement;

  function fill(token) { return { fill: "var(--" + token + ")" }; }
  function P(d, token, extra) { return h("path", Object.assign({ d: d, style: fill(token) }, extra)); }
  function R(x, y, w, ht, rx, token, extra) { return h("rect", Object.assign({ x: x, y: y, width: w, height: ht, rx: rx, style: fill(token) }, extra)); }
  function C(cx, cy, r, token, extra) { return h("circle", Object.assign({ cx: cx, cy: cy, r: r, style: fill(token) }, extra)); }
  function El(cx, cy, rx, ry, token, rotate) {
    return h("ellipse", { cx: cx, cy: cy, rx: rx, ry: ry, style: fill(token), transform: rotate ? "rotate(" + rotate + " " + cx + " " + cy + ")" : undefined });
  }
  function cx() { return Array.prototype.filter.call(arguments, Boolean).join(" "); }

  /* The mark, drawn from the logo's master geometry on a 120 box. */
  function Mark(props) {
    var mono = props.mono;
    var dot = mono ? "bird-line" : "bird-dot";
    var dash = mono ? "bird-line" : "bird-dash";
    var eyes = mono ? "surface" : "bird-eye";
    return h("svg", { viewBox: "0 0 120 120", width: props.size, height: props.size, role: props.label ? "img" : undefined, "aria-label": props.label, "aria-hidden": props.label ? undefined : "true", style: { flex: "none" } },
      R(4, 86, 112, 6, 3, "bird-line"),
      P("M18 50 L2 46 L16 68 Z", dot),
      C(38, 62, 24, dot),
      P("M59 54 L72 60 L59 66 Z", "bird-line"),
      C(46, 54, 6.5, eyes),
      mono ? null : C(48, 54, 3.2, "bird-pupil"),
      P("M100 58 L116 74 L100 78 Z", dash),
      R(74, 22, 30, 64, 15, dash),
      P("M75 34 L63 40 L75 46 Z", "bird-line"),
      C(84, 38, 6.5, eyes),
      mono ? null : C(82, 38, 3.2, "bird-pupil"),
      h("g", { transform: "rotate(-8 57 20)" },
        R(46, 12, 22, 16, 2, "mark-letter"),
        P("M46 13.5 L57 21 L68 13.5 L68 16 L57 23.5 L46 16 Z", "mark-fold")
      )
    );
  }

  function Logo(props) {
    var variant = props.variant || "lockup";
    var size = props.size || (variant === "icon" ? 72 : variant === "lockup" ? 64 : 120);
    var label = "birdie";

    if (variant === "icon") {
      var body;
      if (size >= 56) {
        body = h(Mark, { size: size * 0.84 });
      } else if (size >= 32) {
        body = h("svg", { viewBox: "0 0 120 120", width: size * 0.84, height: size * 0.84, "aria-hidden": "true" },
          R(4, 86, 112, 8, 4, "bird-line"),
          C(34, 60, 26, "bird-dot"),
          C(42, 52, 8, "bird-eye"),
          R(80, 20, 32, 66, 16, "bird-dash"),
          C(90, 38, 8, "bird-eye"),
          h("g", { transform: "rotate(-8 59 18)" },
            R(42, 6, 34, 24, 3, "mark-letter"),
            P("M42 9 L59 20 L76 9 L76 13 L59 24 L42 13 Z", "mark-fold")
          )
        );
      } else {
        body = h("svg", { viewBox: "0 0 24 24", width: size, height: size, "aria-hidden": "true" },
          C(9, 14, 5.5, "bird-dot"),
          R(15, 4.5, 6.5, 15, 3.25, "bird-dash")
        );
      }
      return h("span", { className: cx("bd-icon", props.className), "data-theme": "light", role: "img", "aria-label": label, style: { width: size, height: size, borderRadius: Math.round(size * 0.24) } }, body);
    }

    if (variant === "mark") {
      return h(Mark, { size: size, label: label, mono: props.mono });
    }

    return h("span", { className: cx("bd-logo", props.className), role: "img", "aria-label": label, style: { gap: Math.round(size * 0.07) } },
      h(Mark, { size: size, mono: props.mono }),
      h("span", { className: "bd-logo-word", style: { fontSize: Math.round(size * 0.45) } }, "birdie"),
      props.signal === false ? null : h("span", { className: "bd-logo-dot", style: { width: Math.round(size * 0.075), height: Math.round(size * 0.075) } }),
      props.signal === false ? null : h("span", { className: "bd-logo-dash", style: { width: Math.round(size * 0.18), height: Math.round(size * 0.075) } })
    );
  }

  /* Bird parts, on the 240 x 150 pose grid with the wire at y = 124. */
  function DotBird(props) {
    return h("g", { transform: props.transform },
      P("M42 76 L20 68 L40 104 Z", "bird-dot"),
      C(70, 92, 32, "bird-dot"),
      P("M100 82 L116 90 L100 98 Z", "bird-line"),
      h("g", { className: props.openClass, opacity: props.happy ? 0 : undefined }, C(82, 80, 9, "bird-eye"), C(85, 80, 4.5, "bird-pupil")),
      props.happy || props.happyClass ? P("M74 84 Q83 68 92 84 Q83 77 74 84 Z", "bird-line", { className: props.happyClass }) : null,
      El(58, 98, 14, 10, "bird-dot-wing", -20)
    );
  }
  function DashBird(props) {
    return h("g", { transform: props.transform },
      P("M186 88 L206 114 L186 118 Z", "bird-dash"),
      R(150, 40, 40, 84, 20, "bird-dash"),
      P("M151 58 L135 66 L151 74 Z", "bird-line"),
      h("g", { className: props.awakeClass }, C(164, 62, 9, "bird-eye"), C(161, 62, 4.5, "bird-pupil")),
      props.sleepClass ? h("g", { className: props.sleepClass }, R(155, 60, 18, 4, 2, "bird-eye"), P("M128 16 h12 v3 l-8 9 h8 v3 h-12 v-3 l8 -9 h-8 z", "bird-line")) : null,
      El(174, 94, 9, 16, "bird-dash-wing")
    );
  }
  function LetterShape(x, y, w, ht, fold, cls) {
    var b = Math.max(1.5, ht * 0.08);
    return h("g", { className: cls },
      R(x, y, w, ht, ht * 0.1, "letter-edge"),
      R(x + b, y + b, w - b * 2, ht - b * 2, ht * 0.05, "letter"),
      P("M" + (x + b) + " " + (y + b * 1.6) + " L" + (x + w / 2) + " " + (y + ht * 0.56) + " L" + (x + w - b) + " " + (y + b * 1.6) +
        " L" + (x + w - b) + " " + (y + b * 1.6 + ht * 0.13) + " L" + (x + w / 2) + " " + (y + ht * 0.69) + " L" + (x + b) + " " + (y + b * 1.6 + ht * 0.13) + " Z", fold)
    );
  }

  var POSE_LABEL = {
    ask: "Dot leans in and pushes a letter along the wire toward Dash",
    idle: "A letter reaches Dash, who is lying asleep on the wire",
    reply: "Dash bends over and pushes the answer back along the wire",
    loop: "Looping animation: Dot pushes a letter along the wire. Dash wakes, thinks and pushes the answer back. Dot hops."
  };

  function Birds(props) {
    var pose = props.pose || "loop";
    var svg;
    var common = { className: "bd-birds", role: "img", "aria-label": POSE_LABEL[pose] };
    if (pose === "ask") {
      svg = h("svg", Object.assign({ viewBox: "0 0 240 150" }, common),
        R(-10, 124, 260, 6, 0, "bird-line"),
        h(DotBird, { transform: "rotate(14 70 124)" }),
        h(DashBird, {}),
        LetterShape(118, 106, 24, 18, "letter-fold-request")
      );
    } else if (pose === "idle") {
      svg = h("svg", Object.assign({ viewBox: "0 0 200 150" }, common),
        R(-10, 124, 220, 6, 0, "bird-line"),
        h("g", { transform: "translate(230 0) scale(-1 1)" },
          P("M66 92 L40 84 L64 112 Z", "bird-dash"),
          R(60, 84, 110, 40, 20, "bird-dash"),
          El(106, 106, 20, 9, "bird-dash-wing"),
          R(136, 98, 16, 4, 2, "bird-eye"),
          P("M168 96 L184 104 L168 112 Z", "bird-line")
        ),
        LetterShape(18, 106, 24, 18, "letter-fold-request"),
        P("M76 50 h14 v3 l-10 11 h10 v3 h-14 v-3 l10 -11 h-10 z", "bird-line"),
        P("M58 30 h10 v2.4 l-7 7.6 h7 v2.4 h-10 v-2.4 l7 -7.6 h-7 z", "bird-line")
      );
    } else if (pose === "reply") {
      svg = h("svg", Object.assign({ viewBox: "90 0 165 150" }, common),
        R(80, 124, 185, 6, 0, "bird-line"),
        h(DashBird, { transform: "rotate(-28 170 124)" }),
        LetterShape(108, 106, 24, 18, "letter-fold-reply")
      );
    } else {
      svg = h("svg", Object.assign({ viewBox: "130 20 860 420" }, common),
        R(100, 360, 920, 8, 0, "bird-line"),
        LetterShape(440, 298, 90, 62, "letter-fold-request", "bd-loop-req"),
        LetterShape(630, 298, 90, 62, "letter-fold-reply", "bd-loop-rep"),
        h("g", { className: "bd-loop-sig" }, C(600, 186, 14, "brand-tomato"), R(624, 174, 52, 24, 12, "bird-dash")),
        h("g", { className: "bd-loop-dot" }, h(DotBird, { transform: "translate(120 -12) scale(3)", openClass: "bd-loop-dot-open", happyClass: "bd-loop-dot-happy" })),
        h("g", { className: "bd-loop-dash" }, h(DashBird, { transform: "translate(320 -12) scale(3)", awakeClass: "bd-loop-dash-awake", sleepClass: "bd-loop-dash-sleep" }))
      );
    }
    if (props.stage === false) return svg;
    return h("div", { className: cx("bd-birds-stage", props.className) }, svg);
  }

  function Letter(props) {
    var tone = props.tone || "request";
    var w = props.size || 24;
    var ht = w * 0.75;
    var body = tone === "solid"
      ? [R(0, 0, 24, 18, 2, "mark-letter", { key: "a" }), P("M0 2 L12 11 L24 2 L24 5 L12 14 L0 5 Z", "mark-fold", { key: "b" })]
      : [h("g", { key: "c" }, LetterShape(0, 0, 24, 18, tone === "reply" ? "letter-fold-reply" : "letter-fold-request"))];
    return h("svg", { viewBox: "0 0 24 18", width: w, height: ht, role: props.label ? "img" : undefined, "aria-label": props.label, "aria-hidden": props.label ? undefined : "true", style: { flex: "none", display: "block" } }, body);
  }

  function Button(props) {
    var kind = props.kind || "primary";
    var rest = Object.assign({}, props);
    delete rest.kind; delete rest.children; delete rest.className;
    var cls = cx("bd-button", "bd-button-" + kind, props.className);
    if (props.href) return h("a", Object.assign({}, rest, { className: cls }), props.children);
    return h("button", Object.assign({ type: "button" }, rest, { className: cls }), props.children);
  }

  function InstallCommand(props) {
    var command = props.command || "curl -fsSL https://raw.githubusercontent.com/koshikraj/birdie/main/install.sh | sh";
    var state = React.useState("Copy");
    function copy() {
      var done = function () { state[1]("Copied"); };
      var fail = function () { state[1]("Select to copy"); };
      try {
        if (navigator.clipboard) navigator.clipboard.writeText(command).then(done, fail);
        else fail();
      } catch (e) { fail(); }
    }
    return h("div", { className: cx("bd-install", props.className) },
      h("code", { className: "bd-install-code" }, command),
      h("button", { type: "button", className: "bd-install-copy", onClick: copy }, state[0])
    );
  }

  var fieldCount = 0;
  function TextField(props) {
    var idRef = React.useRef(null);
    if (idRef.current === null) { fieldCount += 1; idRef.current = "bd-field-" + fieldCount; }
    var id = props.id || idRef.current;
    var rest = Object.assign({}, props);
    delete rest.label; delete rest.hint; delete rest.error; delete rest.mono; delete rest.className;
    var noteId = id + "-note";
    return h("div", { className: cx("bd-field", props.className) },
      h("label", { className: "bd-field-label", htmlFor: id }, props.label),
      h("input", Object.assign({ type: "text" }, rest, {
        id: id,
        className: cx("bd-input", props.mono && "bd-input-mono", props.error && "bd-input-error"),
        "aria-invalid": props.error ? "true" : undefined,
        "aria-describedby": props.error || props.hint ? noteId : undefined
      })),
      props.error
        ? h("span", { id: noteId, className: "bd-field-error" }, "Error: " + props.error)
        : props.hint ? h("span", { id: noteId, className: "bd-field-hint" }, props.hint) : null
    );
  }

  var SHAPES = {
    round: function (token) { return C(30, 30, 22, token); },
    pill: function (token) { return R(16, 6, 28, 48, 14, token); },
    arch: function (token) { return P("M10 52 V30 A20 20 0 0 1 50 30 V52 Z", token); },
    square: function (token) { return R(8, 8, 44, 44, 10, token); }
  };
  var SHAPE_ORDER = ["round", "pill", "arch", "square"];
  var SHAPE_FACE = {
    round: { beak: "M50 24 L60 30 L50 36 Z", eye: [38, 24] },
    pill: { beak: "M42 18 L54 24 L42 30 Z", eye: [32, 20] },
    arch: { beak: "M50 24 L60 30 L50 36 Z", eye: [38, 26] },
    square: { beak: "M50 24 L60 30 L50 36 Z", eye: [38, 24] }
  };

  function AgentAvatar(props) {
    var shape = props.shape || "round";
    var size = props.size || 72;
    var face = SHAPE_FACE[shape];
    var token = "agent-" + (SHAPE_ORDER.indexOf(shape) + 1);
    return h("svg", { viewBox: "0 0 64 60", width: size, height: size * 60 / 64, role: props.name ? "img" : undefined, "aria-label": props.name, "aria-hidden": props.name ? undefined : "true", style: { flex: "none", display: "block" } },
      P(face.beak, "bird-line"),
      SHAPES[shape](token),
      props.asleep
        ? R(face.eye[0] - 5, face.eye[1] - 1.5, 10, 3, 1.5, "bird-eye")
        : [C(face.eye[0], face.eye[1], 6, "bird-eye", { key: "w" }), C(face.eye[0] + 2, face.eye[1], 3, "bird-pupil", { key: "p" })]
    );
  }

  function AgentRow(props) {
    return h("button", { type: "button", className: cx("bd-row", props.className), "aria-current": props.selected ? "true" : undefined, onClick: props.onClick },
      h(AgentAvatar, { shape: props.shape, size: 30, asleep: props.asleep }),
      h("span", { className: "bd-row-name" }, props.name),
      props.unread ? h("span", { className: "bd-row-unread", "aria-label": props.unread + " unread" }, h(Letter, { size: 14, tone: "request" }), props.unread) : null
    );
  }

  function MessageBubble(props) {
    var side = props.side || "in";
    return h("div", { className: cx("bd-message", "bd-message-" + side, props.className) },
      h("div", { className: "bd-message-meta" },
        h("span", null, props.from),
        props.verified ? h("span", { className: "bd-message-verified" },
          h("svg", { viewBox: "0 0 12 12", width: 12, height: 12, "aria-hidden": "true" }, P("M1.5 6 L2.9 4.6 L4.8 6.5 L9.1 2 L10.5 3.4 L4.8 9.3 Z", "ink-muted")),
          "verified sender") : null
      ),
      h("div", { className: cx("bd-bubble", "bd-bubble-" + side) }, props.children)
    );
  }

  function StatusSignal(props) {
    return h("span", { className: cx("bd-signal", props.live !== false && "bd-signal-live", props.className), role: "status" },
      h("span", { className: "bd-signal-marks", "aria-hidden": "true" }, h("span", { className: "bd-signal-dot" }), h("span", { className: "bd-signal-dash" })),
      props.label
    );
  }

  window.Birdie = {
    Logo: Logo,
    Birds: Birds,
    Letter: Letter,
    Button: Button,
    InstallCommand: InstallCommand,
    TextField: TextField,
    AgentAvatar: AgentAvatar,
    AgentRow: AgentRow,
    MessageBubble: MessageBubble,
    StatusSignal: StatusSignal
  };
})();
