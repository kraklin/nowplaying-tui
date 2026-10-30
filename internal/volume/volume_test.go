package volume

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func fakeRunner(out string, err error, got *[]string) Runner {
	return func(_ context.Context, script string) ([]byte, error) {
		if got != nil {
			*got = append(*got, script)
		}
		return []byte(out), err
	}
}

func TestGetParses(t *testing.T) {
	for out, want := range map[string]int{"55,false\n": 55, "0,false": 0, " 100 ,true\n": 100, "250,false": 100, "-3,false": 0, "42": 42} {
		v, err := (&Osascript{run: fakeRunner(out, nil, nil)}).Get()
		if err != nil || v != want {
			t.Errorf("Get(%q) = %d, %v; want %d", out, v, err, want)
		}
	}
}

func TestGetErrors(t *testing.T) {
	if _, err := (&Osascript{run: fakeRunner("missing value,missing value\n", nil, nil)}).Get(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("missing value should be ErrUnavailable, got %v", err)
	}
	if _, err := (&Osascript{run: fakeRunner("hello", nil, nil)}).Get(); err == nil {
		t.Errorf("garbage output should error")
	}
	boom := errors.New("boom")
	if _, err := (&Osascript{run: fakeRunner("", boom, nil)}).Get(); !errors.Is(err, boom) {
		t.Errorf("runner error should pass through, got %v", err)
	}
}

func TestSetScriptAndClamp(t *testing.T) {
	var got []string
	o := &Osascript{run: fakeRunner("", nil, &got)}
	for _, v := range []int{55, -10, 400} {
		if err := o.Set(v); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"set volume output volume 55", "set volume output volume 0", "set volume output volume 100"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("script %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadReportsLevelAndMute(t *testing.T) {
	for out, want := range map[string]struct {
		level int
		muted bool
	}{
		"55,false\n":       {55, false},
		"0,true\n":         {0, true},
		"38,true":          {38, true},
		" 70 , false ":     {70, false},
		"80,missing value": {80, false}, // a level but no mute switch
		"250,true":         {100, true},
	} {
		level, muted, err := (&Osascript{run: fakeRunner(out, nil, nil)}).Read()
		if err != nil || level != want.level || muted != want.muted {
			t.Errorf("Read(%q) = %d, %v, %v; want %d, %v", out, level, muted, err, want.level, want.muted)
		}
	}
	if _, _, err := (&Osascript{run: fakeRunner("missing value,missing value", nil, nil)}).Read(); !errors.Is(err, ErrUnavailable) {
		t.Errorf("no software volume should be ErrUnavailable, got %v", err)
	}
	if _, _, err := (&Osascript{run: fakeRunner("loud,false", nil, nil)}).Read(); err == nil {
		t.Errorf("garbage should error")
	}
}

func TestReadIsOneRoundTrip(t *testing.T) {
	var got []string
	(&Osascript{run: fakeRunner("55,false", nil, &got)}).Read()
	if len(got) != 1 {
		t.Errorf("polling every 2 s must cost one osascript call, not %d: %q", len(got), got)
	}
}

func TestSetMutedScripts(t *testing.T) {
	var got []string
	o := &Osascript{run: fakeRunner("", nil, &got)}
	o.SetMuted(true)
	o.SetMuted(false)
	if want := []string{"set volume output muted true", "set volume output muted false"}; got[0] != want[0] || got[1] != want[1] {
		t.Errorf("scripts %q, want %q", got, want)
	}
	boom := errors.New("boom")
	if err := (&Osascript{run: fakeRunner("", boom, nil)}).SetMuted(true); !errors.Is(err, boom) {
		t.Errorf("errors should pass through, got %v", err)
	}
}

// Compiles the AppleScript we send, without running it, so a syntax mistake is
// caught here and not on someone's machine. Nothing is muted.
func TestScriptsCompile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := exec.LookPath("osacompile"); err != nil {
		t.Skip("no osacompile")
	}
	var scripts []string
	o := &Osascript{run: func(_ context.Context, s string) ([]byte, error) {
		scripts = append(scripts, s)
		return []byte("1,false"), nil
	}}
	o.Read()
	o.Set(50)
	o.SetMuted(true)
	o.SetMuted(false)
	if len(scripts) != 4 {
		t.Fatalf("collected %d scripts", len(scripts))
	}
	for _, s := range scripts {
		out := filepath.Join(t.TempDir(), "x.scpt")
		if b, err := exec.Command("osacompile", "-e", s, "-o", out).CombinedOutput(); err != nil {
			t.Errorf("does not compile: %q: %v %s", s, err, b)
		}
	}
}

func TestFakeMute(t *testing.T) {
	f := NewFake(40)
	if l, m, _ := f.Read(); l != 40 || m {
		t.Errorf("starts unmuted at 40: %d %v", l, m)
	}
	f.SetMuted(true)
	f.Set(60)
	if l, m, _ := f.Read(); l != 60 || !m {
		t.Errorf("mute is independent of the level: %d %v", l, m)
	}
}

func TestFake(t *testing.T) {
	f := NewFake(40)
	f.Set(130)
	if v, _ := f.Get(); v != 100 {
		t.Errorf("fake should clamp, got %d", v)
	}
}

// Reads only; the test never changes your volume.
func TestLiveGet(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS only")
	}
	if _, err := exec.LookPath("osascript"); err != nil {
		t.Skip("no osascript")
	}
	v, err := New().Get()
	if errors.Is(err, ErrUnavailable) {
		t.Skip("output device has no software volume")
	}
	if err != nil || v < 0 || v > 100 {
		t.Errorf("live Get = %d, %v", v, err)
	}
}
