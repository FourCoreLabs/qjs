package qjs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fastschema/qjs"
)

// probe tries every std/os route to the host filesystem and returns what each
// one produced, as a JSON string.
const probe = `
const o = {};
const t = (k, f) => { try { const v = f(); o[k] = (v === null || v === undefined) ? 'null' : String(v); } catch (e) { o[k] = 'ERR ' + e.message; } };
t('loadFile', () => std.loadFile('canary.txt'));
t('open_read', () => { const f = std.open('canary.txt', 'r'); return f ? f.readAsString() : null; });
t('open_write', () => { const f = std.open('pwned.txt', 'w'); return f ? 'OPENED-FOR-WRITE' : null; });
t('readdir', () => JSON.stringify(os.readdir('.')));
JSON.stringify(o);`

func runProbe(t *testing.T, opt qjs.Option) (out string, dir string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "canary.txt"), []byte("HOST-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	rt, err := qjs.New(opt)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	v, err := rt.Context().Eval("probe.js", qjs.Code(probe))
	if err != nil {
		t.Fatal(err)
	}
	defer v.Free()
	return v.String(), dir
}

// Control: the default runtime mounts the working directory, so the probe must
// see the canary. If this ever fails the probe is blind and the next test
// proves nothing.
func TestProbeSeesDefaultHostMount(t *testing.T) {
	out, dir := runProbe(t, qjs.Option{})
	if !strings.Contains(out, "HOST-SECRET") {
		t.Fatalf("control: probe did not read the canary: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned.txt")); err != nil {
		t.Fatalf("control: probe could not write to the host: %v", err)
	}
}

func TestNoHostFSHasNoFilesystemCapability(t *testing.T) {
	out, dir := runProbe(t, qjs.Option{NoHostFS: true})
	if strings.Contains(out, "HOST-SECRET") || strings.Contains(out, "OPENED-FOR-WRITE") {
		t.Fatalf("NoHostFS leaked host access: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned.txt")); err == nil {
		t.Fatal("script created a file on the host")
	}
}

func TestMaxMemoryPagesIsAHardCap(t *testing.T) {
	qjs.MaxMemoryPages = 4096 // 256 MiB
	// The compiled module and its runtime config are cached process-wide;
	// DisableBuildCache forces a rebuild so this test does not depend on order.
	rt, err := qjs.New(qjs.Option{NoHostFS: true, DisableBuildCache: true})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	_, err = rt.Context().Eval("bomb.js", qjs.Code(`const a = new Array(1000000000).fill(0); a.length;`))
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "out of memory") {
		t.Fatalf("expected an out-of-memory error under the page cap, got %v", err)
	}
}
