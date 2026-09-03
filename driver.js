// driver.js — the whole Go build pipeline, client side.
//
// Seeds bottle's jsfs with the std package archives and a main.go, then runs
// the REAL Go compiler and linker (compile.wasm / link.wasm — cmd/compile and
// cmd/link built for js/wasm) as wasm instances against that filesystem, and
// finally instantiates the freshly linked /work/a.wasm. Three "processes",
// one in-memory disk, no server involvement after the assets load.
(function () {
  const logEl = document.getElementById("log");
  function log(line) {
    logEl.textContent += line + "\n";
    try { console.log(line); } catch (_) {}
  }

  // Route the tools' stdout/stderr (jsfs fds 1/2) into the page log.
  let lineBuf = "";
  const sink = (buf) => {
    lineBuf += new TextDecoder().decode(buf);
    let i;
    while ((i = lineBuf.indexOf("\n")) >= 0) {
      log("  | " + lineBuf.slice(0, i));
      lineBuf = lineBuf.slice(i + 1);
    }
  };
  jsfs.stdio.stdout = sink;
  jsfs.stdio.stderr = sink;

  async function fetchBytes(url) {
    const r = await fetch(url);
    if (!r.ok) throw new Error(url + ": " + r.status);
    return new Uint8Array(await r.arrayBuffer());
  }

  async function seed() {
    const man = await (await fetch("manifest.json")).json();
    let n = 0, bytes = 0;
    for (const p of man.files) {
      const b = await fetchBytes(p);
      jsfs.writeFile("/" + p, b);
      n++; bytes += b.length;
    }
    jsfs.writeFile("/importcfg", await fetchBytes("importcfg"));
    jsfs.writeFile("/importcfg.link", await fetchBytes("importcfg.link"));
    jsfs.mkdirp("/src"); jsfs.mkdirp("/work");
    const src = await fetchBytes("hello/main.go");
    jsfs.writeFile("/src/main.go", src);
    document.getElementById("src").value = new TextDecoder().decode(src);
    log("seeded " + n + " package archives (" + (bytes >> 20) + " MB) + importcfgs + /src/main.go");
  }

  const moduleCache = {};
  async function tool(url, argv, env) {
    if (!moduleCache[url]) moduleCache[url] = fetchBytes(url);
    const bytes = await moduleCache[url];
    const go = new Go();
    go.argv = argv;
    go.env = env || { GOOS: "js", GOARCH: "wasm", GOROOT: "/goroot", HOME: "/root", TMPDIR: "/tmp", PATH: "/bin" };
    let code = 0;
    go.exit = (c) => { code = c; };
    const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
    const t0 = performance.now();
    await go.run(instance);
    return { code: code, ms: Math.round(performance.now() - t0) };
  }

  let seeded = false;
  async function main() {
    try {
      log("== gotab: the Go toolchain, in this tab ==");
      if (!seeded) { await seed(); seeded = true; }
      jsfs.writeFile("/src/main.go", new TextEncoder().encode(document.getElementById("src").value));

      log("compile.wasm: compiling /src/main.go …");
      let r = await tool("compile.wasm",
        ["compile", "-p", "main", "-pack", "-importcfg", "/importcfg", "-o", "/work/main.a", "/src/main.go"]);
      if (r.code !== 0) { log("COMPILE FAILED rc=" + r.code); return; }
      log("compiled → /work/main.a (" + jsfs.readFile("/work/main.a").length + " bytes, " + r.ms + " ms)");

      log("link.wasm: linking …");
      r = await tool("link.wasm",
        ["link", "-importcfg", "/importcfg.link", "-buildmode=exe", "-o", "/work/a.wasm", "/work/main.a"]);
      if (r.code !== 0) { log("LINK FAILED rc=" + r.code); return; }
      const prog = jsfs.readFile("/work/a.wasm");
      log("linked → /work/a.wasm (" + prog.length + " bytes, " + r.ms + " ms)");

      log("running /work/a.wasm …");
      const go = new Go();
      go.argv = ["a.wasm"];
      const { instance } = await WebAssembly.instantiate(prog, go.importObject);
      await go.run(instance);
      log("== done: the program above was compiled, linked and executed entirely in this tab ==");
    } catch (e) {
      log("DRIVER ERROR: " + (e && (e.stack || e.message || e)));
    }
  }
  document.getElementById("build").onclick = function () { logEl.textContent = ""; main(); };
  main();
})();
