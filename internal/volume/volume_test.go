package volume

import (
	"context"
	"errors"
	"os/exec"
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
	for out, want := range map[string]int{"55\n": 55, "0": 0, " 100 \n": 100, "250": 100, "-3": 0} {
		v, err := (&Osascript{run: fakeRunner(out, nil, nil)}).Get()
		if err != nil || v != want {
			t.Errorf("Get(%q) = %d, %v; want %d", out, v, err, want)
		}
	}
}

func TestGetErrors(t *testing.T) {
	if _, err := (&Osascript{run: fakeRunner("missing value\n", nil, nil)}).Get(); !errors.Is(err, ErrUnavailable) {
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
